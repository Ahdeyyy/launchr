package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

type rate struct {
	Date    string    `json:"date"`
	Base    string    `json:"base"`
	Quote   string    `json:"quote"`
	Rate    float64   `json:"rate"`
	Fetched time.Time `json:"fetched"`
}

// Stale rates are usable immediately. Refreshes belong to the app lifetime,
// not a particular amount edit, and are rate-limited per currency pair.
func (a *app) currencyResult(ctx context.Context, q conversion, client *http.Client, endpoint string) (string, string, error) {
	var cached rate
	key := q.From + "/" + q.To
	if q.From == q.To || a.db.load("rates", key, &cached) != nil || cached.Rate <= 0 || time.Since(cached.Fetched) <= 6*time.Hour {
		return convertCurrency(ctx, a.db, client, endpoint, q)
	}
	value := q.Amount * cached.Rate
	if math.IsInf(value, 0) {
		return "", "", fmt.Errorf("amount is too large")
	}
	return fmt.Sprintf("%.2f %s", value, q.To), "Cached rate from " + cached.Date + " (refreshing when available)", nil
}

func (a *app) refreshCurrency(q conversion, client *http.Client, endpoint string) {
	key := q.From + "/" + q.To
	a.rateMu.Lock()
	if a.rateRetry == nil {
		a.rateRetry = make(map[string]time.Time)
	}
	start := time.Now().After(a.rateRetry[key]) && a.ctx != nil && a.ctx.Err() == nil
	if start {
		a.rateRetry[key] = time.Now().Add(time.Minute)
	}
	a.rateMu.Unlock()
	if start {
		a.background(func() {
			job, cancel := context.WithTimeout(a.ctx, 8*time.Second)
			defer cancel()
			_, detail, err := convertCurrency(job, a.db, client, endpoint, q)
			a.update(func() {
				current, ok := parseConversion(a.query)
				if !ok || current.From != q.From || current.To != q.To || (a.tab != "Apps" && a.tab != "Calculator") || a.ctx.Err() != nil {
					return
				}
				if err != nil {
					a.status = err.Error()
					return
				}
				if strings.HasPrefix(detail, "Offline") {
					for i := range a.rows {
						a.rows[i].Detail = detail
					}
					return
				}
				a.search(false)
			})
		})
	}
}

func convertCurrency(ctx context.Context, s *store, client *http.Client, endpoint string, q conversion) (string, string, error) {
	if q.From == q.To {
		return fmt.Sprintf("%.2f %s", q.Amount, q.To), "Same currency", nil
	}
	key := q.From + "/" + q.To
	var cached rate
	hasCache := s.load("rates", key, &cached) == nil && cached.Rate > 0
	r := cached
	stale := false
	if !hasCache || time.Since(cached.Fetched) > 6*time.Hour {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/"+q.From+"/"+q.To, nil)
		if err != nil {
			return "", "", err
		}
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				err = fmt.Errorf("no rate available for %s → %s", q.From, q.To)
			} else {
				err = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&r)
				if err == nil && (r.Rate <= 0 || math.IsInf(r.Rate, 0) || r.Base != q.From || r.Quote != q.To) {
					err = fmt.Errorf("invalid exchange rate response")
				}
			}
		}
		if err != nil {
			if !hasCache {
				return "", "", err
			}
			r = cached
			stale = true
		} else {
			r.Fetched = time.Now()
			if err = s.save("rates", key, r); err != nil {
				return "", "", err
			}
		}
	}
	value := q.Amount * r.Rate
	if math.IsInf(value, 0) {
		return "", "", fmt.Errorf("amount is too large")
	}
	detail := "Rate from " + r.Date + "   Frankfurter"
	if stale {
		detail = "Offline cached rate from " + r.Date
	}
	return fmt.Sprintf("%.2f %s", value, q.To), detail, nil
}
func obsidianArgs(vault, command string, params ...string) []string {
	args := []string{}
	if vault != "" {
		args = append(args, "vault="+vault)
	}
	return append(append(args, command), params...)
}
func runObsidian(ctx context.Context, vault, command string, params ...string) (string, error) {
	path, err := exec.LookPath("obsidian")
	if err != nil {
		return "", fmt.Errorf("install and enable the Obsidian CLI, then restart Launchr")
	}
	cmd := exec.CommandContext(ctx, path, obsidianArgs(vault, command, params...)...)
	hideCommand(cmd)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		if strings.Contains(string(out), "unable to find Obsidian") {
			return "", fmt.Errorf("open Obsidian, then retry this action")
		}
		return "", fmt.Errorf("Obsidian: %s (%v)", strings.TrimSpace(string(out)), err)
	}
	text := strings.TrimSpace(string(out))
	if strings.HasPrefix(text, "Error:") {
		return "", fmt.Errorf("%s", text)
	}
	return text, nil
}
