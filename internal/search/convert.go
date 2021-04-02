package search

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/go-enry/go-enry/v2"
	"github.com/sourcegraph/sourcegraph/internal/search/filter"
	"github.com/sourcegraph/sourcegraph/internal/search/query"
)

func unionRegexp(values []string) string {
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
	for _, v := range values {
		values = append(values, f(v))
	}
	return values
}

const defaultMaxSearchResults = 30
const defaultMaxSearchResultsStreaming = 500

// Handle pagination count later
func count(q query.Basic, p Protocol, isStructural bool) int {
	if count := q.GetCount(); count != "" {
		v, _ := strconv.Atoi(count) // Invariant: count is validated.
		return v
	}

	if isStructural {
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
	filesInclude = appendMap(langInclude, langToFileRegexp)
	filesExclude = appendMap(langExclude, langToFileRegexp)
	filesReposMustInclude, filesReposMustExclude := q.IncludeExcludeValues(query.FieldRepoHasFile)
	selector, _ := filter.SelectPathFromString(q.FindValue(query.FieldSelect)) // Invariant: already validated.

	isStructural := q.IsStructural()
	count := count(q, p, isStructural)

	return &TextPatternInfo{
		// Atomic Assumptions
		IsRegExp:        q.IsRegexp(),
		IsStructuralPat: isStructural,
		IsCaseSensitive: q.IsCaseSensitive(),
		FileMatchLimit:  int32(count),
		Pattern:         q.Pattern.(query.Pattern).Value,
		IsNegated:       q.Pattern.(query.Pattern).Negated,

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
