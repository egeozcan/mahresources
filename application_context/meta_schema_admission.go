package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"mahresources/shortcodes"
)

// jsonschema (santhosh-tekuri/jsonschema/v6) compiles without a context or work
// budget and keeps every numeric constraint as an exact big.Rat. A 15 KB schema
// of 500 "minimum": 1e-1000000 retained about 200 MB, and plain json.Unmarshal
// lets the value through as zero. The compiler cannot be interrupted once it
// starts, so what it may be handed is admitted first, on a streaming pass that
// builds no tree: numbers, nesting, size and the regular expressions it will
// expand.
const (
	maxSchemaNumberExponent = 400
	maxSchemaSignificant    = 100
	maxSchemaTotalDigits    = 10_000
	maxSchemaDepth          = 64
	maxSchemaTokens         = 500_000
)

var errSchemaNotAdmitted = errors.New("meta JSON schema is outside the supported limits")

// admitMetaSchema walks schemaJSON as a token stream and refuses what the
// compiler must not see. A document that is not valid JSON is not refused here:
// the compiler's own parse reports it.
func admitMetaSchema(schemaJSON string) error {
	dec := json.NewDecoder(strings.NewReader(schemaJSON))
	dec.UseNumber()

	type frame struct {
		object    bool
		name      string // the key this container sits under in its parent
		key       string // the current key, in an object
		expectKey bool
	}
	var stack []frame
	totalDigits, tokens := 0, 0

	valueDone := func() {
		if n := len(stack); n > 0 && stack[n-1].object {
			stack[n-1].expectKey = true
		}
	}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return nil // malformed: the compiler's parse says where
		}
		if tokens++; tokens > maxSchemaTokens {
			return fmt.Errorf("%w: more than %d values", errSchemaNotAdmitted, maxSchemaTokens)
		}
		switch t := tok.(type) {
		case json.Delim:
			switch t {
			case '{', '[':
				name := ""
				if n := len(stack); n > 0 && stack[n-1].object {
					name = stack[n-1].key
				}
				if len(stack) >= maxSchemaDepth {
					return fmt.Errorf("%w: nested more than %d levels", errSchemaNotAdmitted, maxSchemaDepth)
				}
				stack = append(stack, frame{object: t == '{', name: name, expectKey: t == '{'})
			default:
				stack = stack[:len(stack)-1]
				valueDone()
			}
		case string:
			n := len(stack)
			switch {
			case n > 0 && stack[n-1].object && stack[n-1].expectKey:
				stack[n-1].key, stack[n-1].expectKey = t, false
				if stack[n-1].name == "patternProperties" {
					if err := shortcodes.AdmitRegexp(t); err != nil {
						return fmt.Errorf("%w: a patternProperties pattern is too complex", errSchemaNotAdmitted)
					}
				}
			default:
				if n > 0 && stack[n-1].object && stack[n-1].key == "pattern" {
					if err := shortcodes.AdmitRegexp(t); err != nil {
						return fmt.Errorf("%w: a pattern is too complex to compile", errSchemaNotAdmitted)
					}
				}
				valueDone()
			}
		case json.Number:
			if err := admitNumber(string(t), &totalDigits); err != nil {
				return err
			}
			valueDone()
		default:
			valueDone()
		}
	}
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
