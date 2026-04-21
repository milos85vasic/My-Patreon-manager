package definitions

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/milos85vasic/My-Patreon-Manager/internal/envwizard/core"
)

func ValidateValue(v *core.EnvVar, value string) error {
	if value == "" {
		if v.Required {
			return fmt.Errorf("%s is required", v.Name)
		}
		return nil
	}

	switch v.Validation {
	case core.ValidationPort:
		return validatePort(value)
	case core.ValidationURL:
		return validateURL(value)
	case core.ValidationBoolean:
		return validateBoolean(value)
	case core.ValidationNumber:
		return validateNumber(value)
	case core.ValidationCustom:
		return validateCustom(value, v.ValidationRule)
	case core.ValidationToken:
		if len(value) < 8 {
			return fmt.Errorf("%s must be at least 8 characters", v.Name)
		}
	case core.ValidationRequired:
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", v.Name)
		}
	}
	return nil
}

func ValidateAll(vars []*core.EnvVar, values map[string]string) map[string]error {
	errs := make(map[string]error)
	for _, v := range vars {
		val := values[v.Name]
		if err := ValidateValue(v, val); err != nil {
			errs[v.Name] = err
		}
	}
	return errs
}

func validatePort(value string) error {
	port, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid port number: %s", value)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", port)
	}
	return nil
}

func validateURL(value string) error {
	u, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("invalid URL: %s", value)
	}
	if u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("URL must include scheme and host (e.g. http://localhost:8080)")
	}
	return nil
}

func validateBoolean(value string) error {
	switch strings.ToLower(value) {
	case "true", "false", "1", "0", "yes", "no":
		return nil
	}
	return fmt.Errorf("invalid boolean value: %s (expected true/false, 1/0, yes/no)", value)
}

func validateNumber(value string) error {
	_, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("invalid number: %s", value)
	}
	return nil
}

func validateCustom(value, rule string) error {
	if rule == "" {
		return nil
	}
	re, err := regexp.Compile(rule)
	if err != nil {
		return fmt.Errorf("invalid validation rule: %s", rule)
	}
	if !re.MatchString(value) {
		return fmt.Errorf("value %q does not match rule %s", value, rule)
	}
	return nil
}
