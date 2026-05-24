package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"

	domeneshopProvider "github.com/vidarno/external-dns-domeneshop-webhook/internal/provider"
)

const (
	mediaTypeFormat      = "application/external.dns.webhook+json;"
	contentTypeHeader    = "Content-Type"
	contentTypePlaintext = "text/plain"
	acceptHeader         = "Accept"
	varyHeader           = "Vary"
)

// Webhook for external dns provider
type Webhook struct {
	provider     domeneshopProvider.Provider
	domainFilter *DomainFilter
}

// New creates a new instance of the Webhook
func New(apiToken, apiSecret string) *Webhook {
	p := domeneshopProvider.NewProvider(apiToken, apiSecret)
	return &Webhook{provider: *p, domainFilter: loadDomainFilterFromEnv()}
}

func (p *Webhook) AdjustEndpoints(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Received %s request for %s\n", r.Method, r.URL.Path)
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()

	out, err := p.provider.AdjustEndpoints(r.Body)
	if err != nil {
		w.Header().Set(contentTypeHeader, contentTypePlaintext)
		w.WriteHeader(http.StatusBadRequest)
		errMessage := fmt.Sprintf("failed to decode request body: %v", err)
		if _, writeError := fmt.Fprint(w, errMessage); writeError != nil {
			fmt.Printf("error writing error message to response writer")
		}
		return
	}

	w.Header().Set(contentTypeHeader, string(mediaTypeFormat+"version="+"1"))
	w.Header().Set(varyHeader, contentTypeHeader)
	if _, writeError := fmt.Fprint(w, string(out)); writeError != nil {
		fmt.Printf("error writing response")
	}
}

// Records handles the GET request for Records and sends POST request for ApplyChanges to applyChanges-function
func (p *Webhook) Records(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Received %s request for %s\n", r.Method, r.URL.Path)

	switch r.Method {
	case http.MethodPost:
		err := p.provider.ApplyChanges(r.Body)
		switch err {
		case "StatusBadRequest":
			w.Header().Set(contentTypeHeader, contentTypePlaintext)
			w.WriteHeader(http.StatusBadRequest)
		case "StatusInternalServerError":
			w.Header().Set(contentTypeHeader, contentTypePlaintext)
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusNoContent)
		}
		return
	case http.MethodGet:
		records, err := p.provider.Records()
		if err != nil {
			w.Header().Set(contentTypeHeader, contentTypePlaintext)
			w.WriteHeader(http.StatusInternalServerError)
			if _, writeError := fmt.Fprint(w, err.Error()); writeError != nil {
				fmt.Printf("error writing error response: %v", writeError)
			}
			return
		}
		w.Header().Set(contentTypeHeader, string(mediaTypeFormat+"version="+"1"))
		w.Header().Set(varyHeader, contentTypeHeader)
		if err := json.NewEncoder(w).Encode(records); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		return
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
}

// DomainFilter returns the negotiated domain filter payload.
func (p *Webhook) DomainFilter(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Received %s request for %s\n", r.Method, r.URL.Path)
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	b, err := p.domainFilterForResponse().MarshalJSON()
	if err != nil {
		fmt.Printf("failed to marshal domain filter, request method: %s, request path: %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set(contentTypeHeader, string(mediaTypeFormat+"version="+"1"))
	if _, writeError := w.Write(b); writeError != nil {
		fmt.Printf("Failure")
	}
}

func (p *Webhook) domainFilterForResponse() *DomainFilter {
	if p.domainFilter == nil {
		return &DomainFilter{}
	}
	return p.domainFilter
}

func loadDomainFilterFromEnv() *DomainFilter {
	filter := &DomainFilter{}
	if include := parseCommaSeparatedEnv("DOMAIN_FILTER", "DOMAIN_FILTER_INCLUDE"); len(include) > 0 {
		filter.Filters = include
	}
	if exclude := parseCommaSeparatedEnv("DOMAIN_FILTER_EXCLUDE", ""); len(exclude) > 0 {
		filter.exclude = exclude
	}

	if regexInclude := strings.TrimSpace(os.Getenv("DOMAIN_FILTER_REGEX_INCLUDE")); regexInclude != "" {
		compiled, err := regexp.Compile(regexInclude)
		if err == nil {
			filter.regex = compiled
		} else {
			fmt.Printf("invalid DOMAIN_FILTER_REGEX_INCLUDE: %v\n", err)
		}
	}
	if regexExclude := strings.TrimSpace(os.Getenv("DOMAIN_FILTER_REGEX_EXCLUDE")); regexExclude != "" {
		compiled, err := regexp.Compile(regexExclude)
		if err == nil {
			filter.regexExclusion = compiled
		} else {
			fmt.Printf("invalid DOMAIN_FILTER_REGEX_EXCLUDE: %v\n", err)
		}
	}

	return filter
}

func parseCommaSeparatedEnv(primary, fallback string) []string {
	env := strings.TrimSpace(os.Getenv(primary))
	if env == "" && fallback != "" {
		env = strings.TrimSpace(os.Getenv(fallback))
	}
	if env == "" {
		return nil
	}

	parts := strings.Split(env, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

type DomainFilter struct {
	regex          *regexp.Regexp
	regexExclusion *regexp.Regexp
	Filters        []string
	exclude        []string
}

type domainFilterSerde struct {
	Include      []string `json:"include,omitempty"`
	Exclude      []string `json:"exclude,omitempty"`
	RegexInclude string   `json:"regexInclude,omitempty"`
	RegexExclude string   `json:"regexExclude,omitempty"`
}

func (df DomainFilter) MarshalJSON() ([]byte, error) {
	if df.regex != nil || df.regexExclusion != nil {
		var include, exclude string
		if df.regex != nil {
			include = df.regex.String()
		}
		if df.regexExclusion != nil {
			exclude = df.regexExclusion.String()
		}
		return json.Marshal(domainFilterSerde{
			RegexInclude: include,
			RegexExclude: exclude,
		})
	}
	sort.Strings(df.Filters)
	sort.Strings(df.exclude)
	return json.Marshal(domainFilterSerde{
		Include: df.Filters,
		Exclude: df.exclude,
	})
}
