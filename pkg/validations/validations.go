package validations

import (
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/asaskevich/govalidator"
	"github.com/pkg/errors"
	"github.com/thoas/go-funk"
)

const (
	baseDomainRegex          = `^[a-z\d]([\-]*[a-z\d]+)+$`
	dnsNameRegex             = `^([a-z\d]([\-]*[a-z\d]+)*\.)+[a-z\d]+([\-]*[a-z\d]+)+$`
	wildCardDomainRegex      = `^(validateNoWildcardDNS\.).+\.?$`
	hostnameRegex            = `^[a-z0-9][a-z0-9\-\.]{0,61}[a-z0-9]$`
	installerArgsValuesRegex = `^[A-Za-z0-9@!#$%*()_+-=//.,";':{}\[\]]+$`
)

var allowedFlags = []string{"--append-karg", "--delete-karg", "-n", "--copy-network", "--network-dir", "--save-partlabel", "--save-partindex", "--image-url", "--image-file"}

func ValidateInstallerArgs(args []string) error {
	argsRe := regexp.MustCompile("^-+.*")
	valuesRe := regexp.MustCompile(installerArgsValuesRegex)

	for _, arg := range args {
		if argsRe.MatchString(arg) {
			if !funk.ContainsString(allowedFlags, arg) {
				return fmt.Errorf("found unexpected flag %s for installer - allowed flags are %v", arg, allowedFlags)
			}
			continue
		}

		if !valuesRe.MatchString(arg) {
			return fmt.Errorf("found unexpected chars in value %s for installer", arg)
		}
	}

	return nil
}

func ValidateDomainNameFormat(dnsDomainName string) (int32, error) {
	domainName := dnsDomainName
	wildCardMatched, wildCardMatchErr := regexp.MatchString(wildCardDomainRegex, dnsDomainName)
	if wildCardMatchErr == nil && wildCardMatched {
		trimmedDomain := strings.TrimPrefix(dnsDomainName, "validateNoWildcardDNS.")
		domainName = strings.TrimSuffix(trimmedDomain, ".")
	}
	matched, err := regexp.MatchString(baseDomainRegex, domainName)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "Single DNS base domain validation for %s", dnsDomainName)
	}
	if matched && len(domainName) > 1 && len(domainName) < 63 {
		return 0, nil
	}
	matched, err = regexp.MatchString(dnsNameRegex, domainName)
	if err != nil {
		return http.StatusInternalServerError, errors.Wrapf(err, "DNS name validation for %s", dnsDomainName)
	}

	if !matched || isDottedDecimalDomain(domainName) || len(domainName) > 255 {
		return http.StatusBadRequest, errors.Errorf(
			"DNS format mismatch: %s domain name is not valid. Must match regex [%s], be no more than 255 characters, and not be in dotted decimal format (##.##.##.##)",
			dnsDomainName, dnsNameRegex)
	}
	return 0, nil
}

func ValidateHostname(name string) error {
	matched, err := regexp.MatchString(hostnameRegex, name)
	if err != nil {
		return errors.Wrapf(err, "Hostname validation for %s", name)
	}
	if !matched {
		return errors.Errorf(`Hostname format mismatch: %s name is not valid.
			Hostname must have a maximum length of 64 characters,
			start and end with a lowercase alphanumerical character,
			and can only contain lowercase alphanumerical characters, dashes, and periods.`, name)
	}
	return nil
}

func AllStrings(vs []string, f func(string) bool) bool {
	for _, v := range vs {
		if !f(v) {
			return false
		}
	}
	return true
}

func ValidateAdditionalNTPSource(commaSeparatedNTPSources string) bool {
	return AllStrings(strings.Split(commaSeparatedNTPSources, ","), ValidateNTPSource)
}

func ValidateNTPSource(ntpSource string) bool {
	if addr := net.ParseIP(ntpSource); addr != nil {
		return true
	}

	if err := ValidateHostname(ntpSource); err == nil {
		return true
	}

	return false
}

// ValidateHTTPFormat validates the HTTP and HTTPS format
func ValidateHTTPFormat(theurl string) error {
	u, err := url.Parse(theurl)
	if err != nil {
		return fmt.Errorf("URL '%s' format is not valid: %w", theurl, err)
	}
	if !(u.Scheme == "http" || u.Scheme == "https") {
		return errors.Errorf("The URL scheme must be http(s) and specified in the URL: '%s'", theurl)
	}
	return nil
}

// ValidateHTTPProxyFormat validates the HTTP Proxy and HTTPS Proxy format
func ValidateHTTPProxyFormat(proxyURL string) error {
	if !govalidator.IsURL(proxyURL) {
		return errors.Errorf("Proxy URL format is not valid: '%s'", proxyURL)
	}
	u, err := url.Parse(proxyURL)
	if err != nil {
		return errors.Errorf("Proxy URL format is not valid: '%s'", proxyURL)
	}
	if u.Scheme == "https" {
		return errors.Errorf("The URL scheme must be http; https is currently not supported: '%s'", proxyURL)
	}
	if u.Scheme != "http" {
		return errors.Errorf("The URL scheme must be http and specified in the URL: '%s'", proxyURL)
	}
	return nil
}

func validateNoProxyEntry(entry string) error {
	s := strings.TrimPrefix(entry, ".")
	if govalidator.IsIP(s) {
		return nil
	}

	if govalidator.IsCIDR(s) {
		return nil
	}

	if govalidator.IsDNSName(s) {
		return nil
	}

	return errors.Errorf("%s is not a valid no_proxy entry", entry)
}

// ValidateNoProxyFormat validates the no-proxy format which should be a comma-separated list
// of destination domain names, domains, IP addresses or other network CIDRs. A domain can be
// prefaced with '.' to include all subdomains of that domain.
func ValidateNoProxyFormat(noProxy string) error {
	if noProxy == "*" {
		return nil
	}
	domains := strings.Split(noProxy, ",")
	dupTracker := map[string]string{}
	for _, s := range domains {
		if _, present := dupTracker[s]; present {
			return errors.Errorf("duplicate no_proxy entry defined: %s", s)
		}

		if err := validateNoProxyEntry(s); err != nil {
			return errors.Wrap(err,
				"NO Proxy is a comma-separated list of destination domain names, domains, IP addresses or other network CIDRs. "+
					"A domain can be prefaced with '.' to include all subdomains of that domain. Use '*' to bypass proxy for all destinations with OpenShift 4.8 or later.")
		}

		dupTracker[s] = ""
	}
	return nil
}

func ValidateTags(tags string) error {
	if tags == "" {
		return nil
	}
	if !AllStrings(strings.Split(tags, ","), IsValidTag) {
		errMsg := "Invalid format for Tags: %s. Tags should be a comma-separated list (e.g. tag1,tag2,tag3). " +
			"Each tag can consist of the following characters: Alphanumeric (aA-zZ, 0-9), underscore (_) and white-spaces."
		return errors.Errorf(errMsg, tags)
	}
	return nil
}

func IsValidTag(tag string) bool {
	tagRegex := `^\w+( \w+)*$` // word characters and whitespace
	return regexp.MustCompile(tagRegex).MatchString(tag)
}

// ValidateCaCertificate ensures the specified base64 CA certificate
// is valid by trying to decode and parse it.
func ValidateCaCertificate(certificate string) error {
	decodedCaCert, err := base64.StdEncoding.DecodeString(certificate)
	if err != nil {
		return errors.Wrap(err, "failed to decode certificate")
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(decodedCaCert); !ok {
		return errors.Errorf("unable to parse certificate")
	}

	return nil
}

// RFC 1123 (https://datatracker.ietf.org/doc/html/rfc1123#page-13)
// states that domains cannot resemble the format ##.##.##.##
func isDottedDecimalDomain(domain string) bool {
	regex := `([\d]+\.){3}[\d]+`
	return regexp.MustCompile(regex).MatchString(domain)
}
