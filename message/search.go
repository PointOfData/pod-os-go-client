package message

// SearchClause represents a search clause
type SearchClause struct {
	Clause string
}

// SearchBranch represents a search branch
type SearchBranch struct {
	Branch string
}

// SearchAction represents a search action
type SearchAction struct {
	Action string
}

// SearchResults represents search results
type SearchResults struct {
	Results []SearchResult
}

// SearchResult represents a single search result
type SearchResult struct {
	TotalEventHits    int
	StartResult       string
	EndResult         string
	ReturnedEventHits int
	SetLinkCount      int
}

// PatternType represents the type of pattern matching operation
type PatternType int

const (
	// Fast pattern matching - character-based pattern match similar to regular expressions
	PatternFastPattern PatternType = iota

	// Wildcard patterns
	PatternQuestionMark // ? - Match any single character
	PatternAsterisk     // * - Trailing wildcard

	// Character set patterns
	PatternCharSet   // [abcdef] - Match any specified in set
	PatternCharRange // [a-z] - Match any within the range

	// Regular expression
	PatternRegexp // Standard (non-extended) regular expression

	// String comparison patterns
	PatternEq // Match strings equal to the low value
	PatternNe // Match strings not equal to the low value
	PatternLe // Match strings less than or equal to the low value
	PatternLt // Match strings less than the low value
	PatternGe // Match strings greater than or equal to the low value
	PatternGt // Match strings greater than the low value

	// Distance-based matching
	PatternDistance // Match strings against Low with edit distance LE to High

	// String range patterns
	PatternRangeEq // Match strings inclusive within low and high values (ge AND le)
	PatternRangeNe // Match strings exclusive that are not within low and high values (lt OR gt)

	// Integer comparison patterns
	PatternIntEq // Integer equality
	PatternIntNe // Integer non-equality
	PatternIntLe // Integer less than or equal
	PatternIntLt // Integer less than
	PatternIntGe // Integer greater than or equal
	PatternIntGt // Integer greater than

	// Integer range patterns
	PatternIntRangeEq // Integer range
	PatternIntRangeNe // Integer range exclusion

	// Double-precision float comparison patterns
	PatternDblEq // Double-precision float equality
	PatternDblNe // Double-precision float non-equality
	PatternDblLe // Double-precision float less than or equal
	PatternDblLt // Double-precision float less than
	PatternDblGe // Double-precision float greater than or equal
	PatternDblGt // Double-precision float greater than

	// Double-precision float range patterns
	PatternDblRangeEq // Double-precision float range
	PatternDblRangeNe // Double-precision float range exclusion
)

// Pattern represents a pattern used for comparison and search operations
// Patterns have a type, as well as low and high match values.
// Generally speaking, the "low" value is always used for single operation patterns
// such as equality or value comparison. Range and exclusion operators always use the "high" value.
type Pattern struct {
	Type      PatternType // The type of pattern matching operation
	LowValue  string      // Used for single operation patterns (equality, comparison)
	HighValue string      // Used for range and exclusion operations
}

// PatternMatch represents the result of a pattern match operation
type PatternMatch struct {
	Matched    bool    // Whether the pattern matched
	Value      string  // The value that was matched
	Position   int     // Position where the match occurred (for string patterns)
	Confidence float64 // Confidence score for the match (0.0 to 1.0)
}

// PatternSearch represents a pattern-based search operation
type PatternSearch struct {
	Patterns      []Pattern // List of patterns to search for
	Operator      string    // Logical operator to combine patterns ("AND", "OR", "NOT")
	CaseSensitive bool      // Whether the search should be case sensitive
	WholeWord     bool      // Whether to match whole words only
}

// FastPattern represents a fast, character-based pattern match
// The native pattern matching system for the AIP
type FastPattern struct {
	Pattern   string // The fast pattern string
	LowValue  string // The low or minimum value to match
	HighValue string // If specified, match all values between low and high
}

// CharSetPattern represents character set matching patterns
type CharSetPattern struct {
	Characters string // Characters to match (e.g., "abcdef" for [abcdef])
	Range      string // Character range (e.g., "a-z" for [a-z])
	Inclusive  bool   // Whether the range is inclusive
}

// DistancePattern represents edit distance-based matching
type DistancePattern struct {
	ComparisonString string // The string to compare against
	MaxDistance      int    // Maximum edit distance allowed
}

// RangePattern represents range-based matching
type RangePattern struct {
	LowValue  string // Lower limit of matches
	HighValue string // High limit of matches
	Inclusive bool   // Whether the range is inclusive
}

