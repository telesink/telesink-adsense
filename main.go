package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/telesink/telesink-go"
	"golang.org/x/oauth2"
	"google.golang.org/api/adsense/v2"
	"google.golang.org/api/option"
)

var pacificTime *time.Location

type State struct {
	Alerts       map[string]bool `json:"alerts"`
	Payments     map[string]bool `json:"payments"`
	PolicyIssues map[string]bool `json:"policy_issues"`
	Sites        map[string]bool `json:"sites"`
	LastDaily    string          `json:"last_daily_earnings_date"`
}

var (
	stateFile = "state.json"
	state     = State{
		Alerts:       make(map[string]bool),
		Payments:     make(map[string]bool),
		PolicyIssues: make(map[string]bool),
		Sites:        make(map[string]bool),
	}
	accountID    = os.Getenv("ADSENSE_ACCOUNT_ID")
	pollInterval = mustParseDuration(os.Getenv("POLL_INTERVAL"), 15*time.Minute)
	oauthClient  *http.Client
)

func mustParseDuration(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Printf("Warning: invalid POLL_INTERVAL %q, using default 15m", s)
		return fallback
	}
	return d
}

func parseAmount(raw string) (currency string, amount float64) {
	raw = strings.TrimSpace(raw)

	if len(raw) > 3 && raw[3] == ' ' {
		candidate := raw[:3]
		rest := strings.TrimSpace(raw[4:])
		if v, err := strconv.ParseFloat(rest, 64); err == nil {
			return candidate, v
		}
	}

	i := 0
	for i < len(raw) && (raw[i] < '0' || raw[i] > '9') {
		i++
	}
	sym := strings.TrimSpace(raw[:i])
	num := strings.TrimSpace(raw[i:])

	symbolMap := map[string]string{
		"$": "USD", "€": "EUR", "£": "GBP", "¥": "JPY", "₹": "INR",
	}
	if code, ok := symbolMap[sym]; ok {
		currency = code
	} else if sym != "" {
		currency = sym
	} else {
		currency = "USD"
	}

	if v, err := strconv.ParseFloat(num, 64); err == nil {
		amount = v
	}
	return
}

func loadState() {
	data, err := os.ReadFile(stateFile)
	if err != nil {
		return
	}
	if err := json.Unmarshal(data, &state); err != nil {
		log.Printf("Warning: corrupt state file, starting fresh: %v", err)
		state = State{
			Alerts:       make(map[string]bool),
			Payments:     make(map[string]bool),
			PolicyIssues: make(map[string]bool),
			Sites:        make(map[string]bool),
		}
	}
}

func saveState() {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		log.Printf("Error marshaling state: %v", err)
		return
	}
	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		log.Printf("Error saving state: %v", err)
	}
}

func pollAdSense() {
	nowPT := time.Now().In(pacificTime)
	today := nowPT.Format("2006-01-02")

	log.Printf("Polling AdSense at %s (PT: %s)", time.Now().UTC().Format(time.RFC3339), today)

	svc, err := adsense.NewService(context.Background(), option.WithHTTPClient(oauthClient))
	if err != nil {
		log.Printf("Error creating AdSense client: %v", err)
		return
	}

	alertsResp, err := svc.Accounts.Alerts.List(accountID).Do()
	if err != nil {
		log.Printf("Error fetching alerts: %v", err)
	} else {
		for _, a := range alertsResp.Alerts {
			if !state.Alerts[a.Name] {
				telesink.Track(telesink.Event{
					Event: "AdSense alert",
					Text:  fmt.Sprintf("[%s] %s", a.Severity, a.Type),
					Emoji: "🚨",
					Properties: map[string]any{
						"severity": a.Severity,
						"type":     a.Type,
						"message":  a.Message,
					},
				})
				state.Alerts[a.Name] = true
			}
		}
	}

	policyResp, err := svc.Accounts.PolicyIssues.List(accountID).Do()
	if err != nil {
		log.Printf("Error fetching policy issues: %v", err)
	} else {
		for _, issue := range policyResp.PolicyIssues {
			if !state.PolicyIssues[issue.Name] {
				telesink.Track(telesink.Event{
					Event: "AdSense policy issue",
					Text:  fmt.Sprintf("%s on %s", issue.Action, issue.Site),
					Emoji: "⚠️",
					Properties: map[string]any{
						"action": issue.Action,
						"site":   issue.Site,
					},
				})
				state.PolicyIssues[issue.Name] = true
			}
		}
	}

	sitesResp, err := svc.Accounts.Sites.List(accountID).Do()
	if err != nil {
		log.Printf("Error fetching sites: %v", err)
	} else {
		for _, site := range sitesResp.Sites {
			key := site.Name + "|" + site.State
			if !state.Sites[key] {
				telesink.Track(telesink.Event{
					Event: "AdSense site status",
					Text:  fmt.Sprintf("%s is now %s", site.Domain, site.State),
					Emoji: "📍",
					Properties: map[string]any{
						"domain": site.Domain,
						"state":  site.State,
					},
				})
				state.Sites[key] = true
			}
		}
	}

	if state.LastDaily == today {
		saveState()
		return
	}

	paymentsResp, err := svc.Accounts.Payments.List(accountID).Do()
	if err != nil {
		log.Printf("Error fetching payments: %v", err)
	} else {
		for _, p := range paymentsResp.Payments {
			if !state.Payments[p.Name] {
				currency, amount := parseAmount(p.Amount)
				text := fmt.Sprintf("Payment of %.2f %s", amount, currency)
				props := map[string]any{
					"amount":   amount,
					"currency": currency,
				}

				if p.Date != nil {
					date := fmt.Sprintf("%04d-%02d-%02d", p.Date.Year, p.Date.Month, p.Date.Day)
					text += " on " + date
					props["date"] = date
				}

				telesink.Track(telesink.Event{
					Event:      "AdSense payment",
					Text:       text,
					Emoji:      "💰",
					Properties: props,
				})
				state.Payments[p.Name] = true
			}
		}
	}

	reportResp, err := svc.Accounts.Reports.Generate(accountID).
		DateRange("YESTERDAY").
		Metrics("ESTIMATED_EARNINGS", "PAGE_VIEWS", "IMPRESSIONS", "CLICKS").
		Do()
	if err != nil {
		log.Printf("Error generating earnings report: %v", err)
	} else if reportResp != nil && len(reportResp.Rows) > 0 {
		row := reportResp.Rows[0]
		if len(row.Cells) >= 4 {
			_, earnings := parseAmount(row.Cells[0].Value)
			currency := "USD"
			if reportResp.Headers != nil {
				for _, h := range reportResp.Headers {
					if h.Name == "ESTIMATED_EARNINGS" && h.CurrencyCode != "" {
						currency = h.CurrencyCode
						break
					}
				}
			}

			pageViews, _ := strconv.ParseInt(row.Cells[1].Value, 10, 64)
			impressions, _ := strconv.ParseInt(row.Cells[2].Value, 10, 64)
			clicks, _ := strconv.ParseInt(row.Cells[3].Value, 10, 64)

			telesink.Track(telesink.Event{
				Event: "AdSense daily earnings",
				Text:  fmt.Sprintf("%.2f %s earned yesterday (%d clicks, %d impressions)", earnings, currency, clicks, impressions),
				Emoji: "📊",
				Properties: map[string]any{
					"earnings":    earnings,
					"currency":    currency,
					"page_views":  pageViews,
					"impressions": impressions,
					"clicks":      clicks,
					"date":        nowPT.AddDate(0, 0, -1).Format("2006-01-02"),
				},
			})
		} else {
			log.Printf("Unexpected earnings report shape: %d cells", len(row.Cells))
		}
	}

	state.LastDaily = today
	saveState()
}

func main() {
	var err error
	pacificTime, err = time.LoadLocation("America/Los_Angeles")
	if err != nil {
		log.Fatalf("Failed to load Pacific timezone: %v", err)
	}

	loadState()
	defer saveState()

	if accountID == "" {
		log.Fatal("ADSENSE_ACCOUNT_ID is required")
	}

	oauthCfg := &oauth2.Config{
		ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
		Endpoint:     oauth2.Endpoint{TokenURL: "https://oauth2.googleapis.com/token"},
		Scopes:       []string{"https://www.googleapis.com/auth/adsense.readonly"},
	}
	token := &oauth2.Token{RefreshToken: os.Getenv("GOOGLE_REFRESH_TOKEN")}
	oauthClient = oauthCfg.Client(context.Background(), token)

	log.Printf("AdSense Telesink poller started (interval: %v)", pollInterval)

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	pollAdSense()
	for range ticker.C {
		pollAdSense()
	}
}
