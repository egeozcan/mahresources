package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"mahresources/shortcodes"
)

// jsonschema (santhosh-tekuri/jsonschema/v6) compiles without a context or work
// budget and keeps every numeric constraint as an exact big.Rat. A 15 KB schema
// of 500 "minimum": 1e-1000000 retained about 200 MB, and plain json.Unmarshal
// lets the value through as zero. The compiler cannot be interrupted once it
// starts, so what it may be handed is admitted first: a streaming pass checks
// numbers, nesting and size, and the compiler's own regexp engine is replaced
// (meterRegexpEngine) by one that adds up the work of every expression the
// compiler actually compiles. There is no shadow traversal of the schema: the
// engine sees exactly the patterns the compiler resolves, through encoded
// pointers, self-document and absolute refs, nested $id bases, $dynamicRef,
// the active dialect, and the metaschema's own regex-format checks.
const (
	maxSchemaNumberExponent = 400
	maxSchemaSignificant    = 100
	maxSchemaTotalDigits    = 10_000
	maxSchemaDepth          = 64
	maxSchemaTokens         = 500_000
)

var errSchemaNotAdmitted = errors.New("meta JSON schema is outside the supported limits")

// admitMetaSchema walks schemaJSON as a token stream and refuses what the
// compiler must not see. It fails closed on a decoding error (the compiler is
// not handed text this pass could not read); the full-document parse that
// follows still validates that the document is complete.
func admitMetaSchema(ctx context.Context, schemaJSON string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(schemaJSON))
	dec.UseNumber()

	depth, totalDigits, tokens := 0, 0, 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Fail closed: the compiler is not handed text this pass could not read.
			return fmt.Errorf("meta JSON schema is not valid JSON: %w", err)
		}
		if tokens++; tokens > maxSchemaTokens {
			return fmt.Errorf("%w: more than %d values", errSchemaNotAdmitted, maxSchemaTokens)
		}
		if tokens&0x3ff == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{', '[':
				if depth++; depth > maxSchemaDepth {
					return fmt.Errorf("%w: nested more than %d levels", errSchemaNotAdmitted, maxSchemaDepth)
				}
			default:
				depth--
			}
		case json.Number:
			if err := admitNumber(string(t), &totalDigits); err != nil {
				return err
			}
		}
	}
	return nil
}

// admitNumber refuses a number whose exact form would be large: an exponent
// outside ±400, more than 100 significant digits, or (summed over the schema)
// more than 10,000 digits.
func admitNumber(lit string, totalDigits *int) error {
	mant, exp := lit, ""
	if i := strings.IndexAny(lit, "eE"); i >= 0 {
		mant, exp = lit[:i], lit[i+1:]
	}
	exp = strings.TrimLeft(exp, "+-")
	digits, significant, seenNonZero := 0, 0, false
	for _, c := range mant {
		if c < '0' || c > '9' {
			continue
		}
		digits++
		if c != '0' {
			seenNonZero = true
		}
		if seenNonZero {
			significant++
		}
	}
	if significant > maxSchemaSignificant {
		return fmt.Errorf("%w: a number has more than %d significant digits", errSchemaNotAdmitted, maxSchemaSignificant)
	}
	if exp != "" {
		trimmed := strings.TrimLeft(exp, "0")
		if len(trimmed) > 4 {
			return fmt.Errorf("%w: a number has an exponent outside +/-%d", errSchemaNotAdmitted, maxSchemaNumberExponent)
		}
		if trimmed != "" {
			if e, err := strconv.Atoi(trimmed); err != nil || e > maxSchemaNumberExponent {
				return fmt.Errorf("%w: a number has an exponent outside +/-%d", errSchemaNotAdmitted, maxSchemaNumberExponent)
			}
		}
	}
	*totalDigits += digits + len(exp)
	if *totalDigits > maxSchemaTotalDigits {
		return fmt.Errorf("%w: the numbers add up to more than %d digits", errSchemaNotAdmitted, maxSchemaTotalDigits)
	}
	return nil
}

// meterRegexpEngine is installed on the compiler before the schema is added. It
// compiles through one shortcodes.RegexAllowance for the whole validation: the
// same rule the template conditionals use (a source estimate before parsing, the
// parse tree's instruction and rune cost before compiling, the compiled program
// measured after), each distinct source counted once, further compiles refused
// once the allowance is spent, and the context checked before each.
func meterRegexpEngine(ctx context.Context) jsonschema.RegexpEngine {
	allowance := shortcodes.NewRegexAllowance(ctx)
	return func(pattern string) (jsonschema.Regexp, error) {
		re, err := allowance.Compile(pattern)
		if err != nil {
			var tooBig *shortcodes.RegexTooComplexError
			if errors.As(err, &tooBig) {
				return nil, fmt.Errorf("%w: %s", errSchemaNotAdmitted, tooBig.Error())
			}
			return nil, err
		}
		return re, nil
	}
}
