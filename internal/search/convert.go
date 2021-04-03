package search

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-enry/go-enry/v2"
	"github.com/sourcegraph/sourcegraph/internal/search/filter"
	"github.com/sourcegraph/sourcegraph/internal/search/query"

	"github.com/inconshreveable/log15"
)

func unionRegexp(values []string) string {
	if len(values) == 0 {
		// Empty string implies nothing for exclude patterns, as opposed to () which
		// means "match empty regex".
		return ""
	}
	if len(values) == 1 {
		// Cosmetic, so that I can diff effectively.
		return values[0]
	}
	return "(" + strings.Join(values, ")|(") + ")"
}

func langToFileRegexp(lang string) string {
	lang, _ = enry.GetLanguageByAlias(lang) // Invariant: already validated.
	extensions := enry.GetLanguageExtensions(lang)
	patterns := make([]string, len(extensions))
	for i, e := range extensions {
		// Add `\.ext$` pattern to match files with the given extension.
		patterns[i] = regexp.QuoteMeta(e) + "$"
	}
	return unionRegexp(patterns)
}

func appendMap(values []string, f func(in string) string) []string {
	var result []string
	for _, v := range values {
		result = append(result, f(v))
	}
	return result
}

const defaultMaxSearchResults = 30
const defaultMaxSearchResultsStreaming = 500

// Handle pagination count later
func count(q query.Basic, p Protocol) int {
	if count := q.GetCount(); count != "" {
		v, _ := strconv.Atoi(count) // Invariant: count is validated.
		return v
	}

	if q.IsStructural() {
		return defaultMaxSearchResults
	}

	switch p {
	case Batch:
		return defaultMaxSearchResults
	case Streaming:
		return defaultMaxSearchResultsStreaming
	case Pagination:
		return math.MaxInt32
	}
	panic("unreachable")
}

type Protocol int

const (
	Streaming Protocol = iota
	Batch
	Pagination
)

// Assumes actually Atomic query -> means we need to expand query.Basic to atomic, or assume atomic.
func ToTextSearch(q query.Basic, p Protocol, transform query.BasicPass) *TextPatternInfo {
	q = transform(q)
	// Handle file: and -file: filters.
	filesInclude, filesExclude := q.IncludeExcludeValues(query.FieldFile)
	// Handle lang: and -lang: filters.
	langInclude, langExclude := q.IncludeExcludeValues(query.FieldLang)
	filesInclude = append(filesInclude, appendMap(langInclude, langToFileRegexp)...)
	filesExclude = append(filesExclude, appendMap(langExclude, langToFileRegexp)...)
	filesReposMustInclude, filesReposMustExclude := q.IncludeExcludeValues(query.FieldRepoHasFile)
	selector, _ := filter.SelectPathFromString(q.FindValue(query.FieldSelect)) // Invariant: already validated.

	vv, _ := json.Marshal(filesInclude)
	log15.Info("x", "1", string(vv))

	count := count(q, p)

	// Gross assumption: for literal searches, the IsRegexp member of
	// TextPatternInfo must be true, and assumes that the literal value is a
	// quoted regexp.
	isRegexp := q.IsLiteral() || q.IsRegexp()

	var pattern string
	if p, ok := q.Pattern.(query.Pattern); ok {
		if q.IsLiteral() {
			pattern = regexp.QuoteMeta(p.Value)
		} else {
			pattern = p.Value
		}
	}

	negated := false
	if p, ok := q.Pattern.(query.Pattern); ok {
		negated = p.Negated
	}

	return &TextPatternInfo{
		// Atomic Assumptions
		IsRegExp:        isRegexp,
		IsStructuralPat: q.IsStructural(),
		IsCaseSensitive: q.IsCaseSensitive(),
		FileMatchLimit:  int32(count),
		Pattern:         pattern,
		IsNegated:       negated,

		// Parameters
		IncludePatterns:              filesInclude,
		FilePatternsReposMustInclude: filesReposMustInclude,
		FilePatternsReposMustExclude: filesReposMustExclude,
		Languages:                    langInclude,
		PathPatternsAreCaseSensitive: false, // Not used in Sourcegraph currently.
		CombyRule:                    q.FindValue(query.FieldCombyRule),
		Index:                        q.Index(),
		Select:                       selector,
		ExcludePattern:               unionRegexp(filesExclude),
	}
}
