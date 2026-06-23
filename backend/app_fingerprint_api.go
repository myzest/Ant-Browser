package backend

import (
	"ant-chrome/backend/internal/fingerprint"
	cryptorand "crypto/rand"
	"math/big"
	"strconv"
	"strings"
	"time"
)

type FingerprintGenerateRequest struct {
	CurrentArgs         []string `json:"currentArgs"`
	ProfileID           string   `json:"profileId"`
	Platform            string   `json:"platform"`
	RegionMode          string   `json:"regionMode"`
	Country             string   `json:"country"`
	Locale              string   `json:"locale"`
	Timezone            string   `json:"timezone"`
	DeviceClass         string   `json:"deviceClass"`
	PreserveUnknownArgs bool     `json:"preserveUnknownArgs"`
	RegenerateSeed      bool     `json:"regenerateSeed"`
}

type FingerprintGenerateResult struct {
	Args     []string                      `json:"args"`
	Warnings []fingerprint.ValidationIssue `json:"warnings"`
	Health   fingerprint.HealthReport      `json:"health"`
	Summary  fingerprint.Summary           `json:"summary"`
}

type FingerprintValidateRequest struct {
	Args        []string `json:"args"`
	ProxyID     string   `json:"proxyId"`
	ProxyConfig string   `json:"proxyConfig"`
}

func (a *App) GenerateFingerprintProfile(request FingerprintGenerateRequest) FingerprintGenerateResult {
	current := fingerprint.ParseArgs(request.CurrentArgs)
	platform := request.Platform
	if strings.TrimSpace(platform) == "" {
		platform = current["--fingerprint-platform"]
	}
	locale := request.Locale
	if strings.TrimSpace(locale) == "" {
		locale = current["--fingerprint-locale"]
	}
	if strings.TrimSpace(locale) == "" {
		locale = current["--lang"]
	}
	timezone := request.Timezone
	if strings.TrimSpace(timezone) == "" {
		timezone = current["--fingerprint-timezone"]
	}
	if strings.TrimSpace(timezone) == "" {
		timezone = current["--timezone"]
	}

	seed := int64(0)
	if request.RegenerateSeed {
		seed = newFingerprintSeed()
	} else if currentSeed := strings.TrimSpace(current["--fingerprint"]); currentSeed != "" {
		if parsed, err := strconv.ParseInt(currentSeed, 10, 64); err == nil && parsed > 0 {
			seed = parsed
		}
	}

	generated, err := fingerprint.Generate(fingerprint.LoadLibrary(), fingerprint.GenerateOptions{
		ProfileID:   request.ProfileID,
		Platform:    platform,
		Country:     request.Country,
		Locale:      locale,
		Timezone:    timezone,
		DeviceClass: request.DeviceClass,
		Seed:        seed,
	})
	if err != nil {
		health := fingerprint.Health(request.CurrentArgs)
		return FingerprintGenerateResult{
			Args:     append([]string{}, request.CurrentArgs...),
			Warnings: append([]fingerprint.ValidationIssue{{Code: "generate_failed", Severity: "red", Field: "fingerprint", Message: err.Error()}}, health.Issues...),
			Health:   health,
		}
	}

	args := fingerprint.Args(generated)
	if request.RegenerateSeed {
		args = append([]string{"--fingerprint=" + strconv.FormatInt(seed, 10)}, args...)
	} else if seed := current["--fingerprint"]; seed != "" {
		args = append([]string{"--fingerprint=" + seed}, args...)
	}
	if request.PreserveUnknownArgs {
		args = append(args, unknownFingerprintArgs(request.CurrentArgs)...)
	}
	health := fingerprint.Health(args)
	return FingerprintGenerateResult{
		Args:     args,
		Warnings: health.Issues,
		Health:   health,
		Summary:  fingerprint.SummaryFor(generated),
	}
}

func (a *App) ValidateFingerprintProfile(request FingerprintValidateRequest) fingerprint.HealthReport {
	return fingerprint.Health(request.Args)
}

func unknownFingerprintArgs(args []string) []string {
	known := fingerprint.ParseArgs(fingerprint.DefaultArgsForOS("windows"))
	known["--fingerprint"] = ""
	out := make([]string, 0)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		key := strings.ToLower(strings.TrimSpace(arg))
		if before, _, ok := strings.Cut(key, "="); ok {
			key = before
		}
		if _, ok := known[key]; !ok && strings.HasPrefix(key, "--") {
			if !strings.Contains(strings.TrimSpace(arg), "=") && i+1 < len(args) {
				next := strings.TrimSpace(args[i+1])
				if next != "" && !strings.HasPrefix(next, "-") {
					out = append(out, strings.TrimSpace(arg)+"="+next)
					i++
					continue
				}
			}
			out = append(out, arg)
		}
	}
	return out
}

func newFingerprintSeed() int64 {
	const maxSeed = int64(2147483647)
	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(maxSeed))
	if err == nil {
		return n.Int64() + 1
	}
	seed := time.Now().UnixNano() % maxSeed
	if seed < 0 {
		seed = -seed
	}
	if seed == 0 {
		seed = 1
	}
	return seed
}
