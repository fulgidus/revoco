package plugins

import (
	"path/filepath"
	"regexp"
	"strings"

	core "github.com/fulgidus/revoco/connectors"
)

// Selector defines criteria for matching data items.
// Selectors can filter items by type, extension, path, metadata, etc.
type Selector struct {
	// By data type
	DataTypes []core.DataType `json:"data_types,omitempty" yaml:"data_types,omitempty"`

	// By file extension (e.g., ".jpg", ".png")
	Extensions []string `json:"extensions,omitempty" yaml:"extensions,omitempty"`

	// By MIME type (supports wildcards like "image/*")
	MimeTypes []string `json:"mime_types,omitempty" yaml:"mime_types,omitempty"`

	// By source connector ID
	SourceConnectors []string `json:"source_connectors,omitempty" yaml:"source_connectors,omitempty"`

	// By path pattern (glob)
	PathMatch   string `json:"path_match,omitempty" yaml:"path_match,omitempty"`
	PathExclude string `json:"path_exclude,omitempty" yaml:"path_exclude,omitempty"`

	// By metadata condition (Lua expression evaluated at runtime)
	Condition string `json:"condition,omitempty" yaml:"condition,omitempty"`

	// Chain from another job's output
	From string `json:"from,omitempty" yaml:"from,omitempty"`

	// Select all items (overrides other filters)
	All bool `json:"all,omitempty" yaml:"all,omitempty"`
}

// SelectorMatcher provides methods to match items against a selector.
type SelectorMatcher struct {
	selector *Selector

	// Compiled patterns (cached)
	pathMatchPattern   *regexp.Regexp
	pathExcludePattern *regexp.Regexp
	mimePatterns       []*mimePattern
}

// mimePattern represents a compiled MIME type pattern.
type mimePattern struct {
	original string
	regex    *regexp.Regexp
}

// NewSelectorMatcher creates a new matcher for the given selector.
func NewSelectorMatcher(s *Selector) (*SelectorMatcher, error) {
	if s == nil {
		return &SelectorMatcher{selector: &Selector{All: true}}, nil
	}

	m := &SelectorMatcher{selector: s}

	// Compile path patterns
	if s.PathMatch != "" {
		pattern, err := globToRegex(s.PathMatch)
		if err != nil {
			return nil, &SelectorError{Field: "path_match", Err: err}
		}
		m.pathMatchPattern = pattern
	}

	if s.PathExclude != "" {
		pattern, err := globToRegex(s.PathExclude)
		if err != nil {
			return nil, &SelectorError{Field: "path_exclude", Err: err}
		}
		m.pathExcludePattern = pattern
	}

	// Compile MIME patterns
	for _, mime := range s.MimeTypes {
		pattern, err := mimeToRegex(mime)
		if err != nil {
			return nil, &SelectorError{Field: "mime_types", Err: err}
		}
		m.mimePatterns = append(m.mimePatterns, &mimePattern{
			original: mime,
			regex:    pattern,
		})
	}

	return m, nil
}

// Match checks if an item matches the selector criteria.
// Note: Condition evaluation requires Lua runtime and is handled separately.
func (m *SelectorMatcher) Match(item *core.DataItem) bool {
	if m.selector == nil || m.selector.All {
		return true
	}

	s := m.selector

	// Check data type
	if len(s.DataTypes) > 0 && !m.matchDataType(item.Type) {
		return false
	}

	// Check extension
	if len(s.Extensions) > 0 && !m.matchExtension(item.Path) {
		return false
	}

	// Check MIME type (if available in metadata)
	if len(s.MimeTypes) > 0 {
		mime, ok := item.Metadata["mime_type"].(string)
		if !ok || !m.matchMimeType(mime) {
			return false
		}
	}

	// Check source connector
	if len(s.SourceConnectors) > 0 && !m.matchSourceConnector(item.SourceConnID) {
		return false
	}

	// Check path match
	if m.pathMatchPattern != nil && !m.pathMatchPattern.MatchString(item.Path) {
		return false
	}

	// Check path exclude
	if m.pathExcludePattern != nil && m.pathExcludePattern.MatchString(item.Path) {
		return false
	}

	// Note: Condition is evaluated by Lua runtime, not here

	return true
}

// MatchWithCondition checks if an item matches, including Lua condition evaluation.
// The conditionEvaluator is a function that evaluates the Lua condition.
func (m *SelectorMatcher) MatchWithCondition(item *core.DataItem, conditionEvaluator func(item *core.DataItem, condition string) (bool, error)) (bool, error) {
	// First check non-Lua conditions
	if !m.Match(item) {
		return false, nil
	}

	// Then check Lua condition if present
	if m.selector.Condition != "" && conditionEvaluator != nil {
		return conditionEvaluator(item, m.selector.Condition)
	}

	return true, nil
}

// matchDataType checks if the item's data type is in the allowed list.
func (m *SelectorMatcher) matchDataType(itemType core.DataType) bool {
	for _, dt := range m.selector.DataTypes {
		if dt == itemType {
			return true
		}
	}
	return false
}

// matchExtension checks if the item's file extension is in the allowed list.
func (m *SelectorMatcher) matchExtension(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range m.selector.Extensions {
		if strings.ToLower(e) == ext {
			return true
		}
	}
	return false
}

// matchMimeType checks if the MIME type matches any of the patterns.
func (m *SelectorMatcher) matchMimeType(mime string) bool {
	for _, pattern := range m.mimePatterns {
		if pattern.regex.MatchString(mime) {
			return true
		}
	}
	return false
}

// matchSourceConnector checks if the item's source connector is in the allowed list.
func (m *SelectorMatcher) matchSourceConnector(connID string) bool {
	for _, id := range m.selector.SourceConnectors {
		if id == connID {
			return true
		}
	}
	return false
}

// ══════════════════════════════════════════════════════════════════════════════
// Helper Functions
// ══════════════════════════════════════════════════════════════════════════════

// globToRegex converts a glob pattern to a regular expression.
func globToRegex(pattern string) (*regexp.Regexp, error) {
	// Escape regex special characters except * and ?
	var result strings.Builder
	result.WriteString("^")

	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				// ** matches any path
				result.WriteString(".*")
				i++ // Skip second *
			} else {
				// * matches anything except /
				result.WriteString("[^/]*")
			}
		case '?':
			result.WriteString("[^/]")
		case '.', '+', '^', '$', '(', ')', '[', ']', '{', '}', '|', '\\':
			result.WriteByte('\\')
			result.WriteByte(c)
		default:
			result.WriteByte(c)
		}
	}

	result.WriteString("$")
	return regexp.Compile(result.String())
}

// mimeToRegex converts a MIME type pattern to a regular expression.
// Supports wildcards like "image/*" or "*/json".
func mimeToRegex(mime string) (*regexp.Regexp, error) {
	// Escape special regex characters, then replace * with .*
	escaped := regexp.QuoteMeta(mime)
	pattern := strings.ReplaceAll(escaped, "\\*", "[^/]+")
	return regexp.Compile("^" + pattern + "$")
}

// MergeSelectors merges multiple selectors into one.
// Later selectors override earlier ones for non-slice fields.
func MergeSelectors(selectors ...*Selector) *Selector {
	if len(selectors) == 0 {
		return nil
	}

	result := &Selector{}

	for _, s := range selectors {
		if s == nil {
			continue
		}

		// Merge slices (append)
		result.DataTypes = append(result.DataTypes, s.DataTypes...)
		result.Extensions = append(result.Extensions, s.Extensions...)
		result.MimeTypes = append(result.MimeTypes, s.MimeTypes...)
		result.SourceConnectors = append(result.SourceConnectors, s.SourceConnectors...)

		// Override strings
		if s.PathMatch != "" {
			result.PathMatch = s.PathMatch
		}
		if s.PathExclude != "" {
			result.PathExclude = s.PathExclude
		}
		if s.Condition != "" {
			result.Condition = s.Condition
		}
		if s.From != "" {
			result.From = s.From
		}

		// Override bool
		if s.All {
			result.All = true
		}
	}

	// Deduplicate slices
	result.DataTypes = dedupeDataTypes(result.DataTypes)
	result.Extensions = dedupeStrings(result.Extensions)
	result.MimeTypes = dedupeStrings(result.MimeTypes)
	result.SourceConnectors = dedupeStrings(result.SourceConnectors)

	return result
}

// dedupeStrings removes duplicates from a string slice.
func dedupeStrings(slice []string) []string {
	if len(slice) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// dedupeDataTypes removes duplicates from a DataType slice.
func dedupeDataTypes(slice []core.DataType) []core.DataType {
	if len(slice) == 0 {
		return nil
	}
	seen := make(map[core.DataType]bool)
	result := make([]core.DataType, 0, len(slice))
	for _, dt := range slice {
		if !seen[dt] {
			seen[dt] = true
			result = append(result, dt)
		}
	}
	return result
}

// IsEmpty returns true if the selector has no criteria set.
func (s *Selector) IsEmpty() bool {
	if s == nil {
		return true
	}
	return len(s.DataTypes) == 0 &&
		len(s.Extensions) == 0 &&
		len(s.MimeTypes) == 0 &&
		len(s.SourceConnectors) == 0 &&
		s.PathMatch == "" &&
		s.PathExclude == "" &&
		s.Condition == "" &&
		s.From == "" &&
		!s.All
}

// Clone creates a deep copy of the selector.
func (s *Selector) Clone() *Selector {
	if s == nil {
		return nil
	}

	clone := &Selector{
		PathMatch:   s.PathMatch,
		PathExclude: s.PathExclude,
		Condition:   s.Condition,
		From:        s.From,
		All:         s.All,
	}

	if len(s.DataTypes) > 0 {
		clone.DataTypes = make([]core.DataType, len(s.DataTypes))
		copy(clone.DataTypes, s.DataTypes)
	}
	if len(s.Extensions) > 0 {
		clone.Extensions = make([]string, len(s.Extensions))
		copy(clone.Extensions, s.Extensions)
	}
	if len(s.MimeTypes) > 0 {
		clone.MimeTypes = make([]string, len(s.MimeTypes))
		copy(clone.MimeTypes, s.MimeTypes)
	}
	if len(s.SourceConnectors) > 0 {
		clone.SourceConnectors = make([]string, len(s.SourceConnectors))
		copy(clone.SourceConnectors, s.SourceConnectors)
	}

	return clone
}
