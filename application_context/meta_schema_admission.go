package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"

	"mahresources/shortcodes"
)

// jsonschema (santhosh-tekuri/jsonschema/v6) compiles without a context or work
// budget and keeps every numeric constraint as an exact big.Rat. A 15 KB schema
// of 500 "minimum": 1e-1000000 retained about 200 MB, and plain json.Unmarshal
// lets the value through as zero. The compiler cannot be interrupted once it
// starts, so what it may be handed is admitted first: a streaming pass checks
// numbers, nesting and size, and a walk of the schema positions adds up the
// work of the regular expressions the compiler will expand.
const (
	maxSchemaNumberExponent = 400
	maxSchemaSignificant    = 100
	maxSchemaTotalDigits    = 10_000
	maxSchemaDepth          = 64
	maxSchemaTokens         = 500_000
	// maxSchemaRegexWork is the sum of the estimated compiled sizes of every
	// pattern and patternProperties key; one pattern is bounded by
	// shortcodes.MaxRegexpProgramSize.
	maxSchemaRegexWork = 200_000
)

var errSchemaNotAdmitted = errors.New("meta JSON schema is outside the supported limits")

// admitMetaSchema walks schemaJSON as a token stream and refuses what the
// compiler must not see. A document that is not valid JSON is not refused here:
// the compiler's own parse reports it.
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
			return nil // malformed: the compiler's parse says where
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
	return admitSchemaRegexes(ctx, schemaJSON)
}

// Keywords whose value is a schema, a list of schemas, or a map of schemas.
// Everything else (const, enum, default, examples, annotations) is data, and a
// member of it named "pattern" is not a pattern.
var (
	schemaKeys     = []string{"items", "additionalProperties", "additionalItems", "not", "if", "then", "else", "contains", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema"}
	schemaListKeys = []string{"allOf", "anyOf", "oneOf", "prefixItems", "items"}
	schemaMapKeys  = []string{"properties", "$defs", "definitions", "dependentSchemas", "dependencies"}
)

// admitSchemaRegexes adds up the compiled size of every pattern at a schema
// position, including schemas reached only through a local $ref.
func admitSchemaRegexes(ctx context.Context, schemaJSON string) error {
	var root any
	if err := json.Unmarshal([]byte(schemaJSON), &root); err != nil {
		return nil
	}
	visited := map[uintptr]bool{}
	var refs []string
	work := int64(0)
	steps := 0

	pattern := func(p string) error {
		n := shortcodes.RegexpProgramSize(p)
		if n > shortcodes.MaxRegexpProgramSize {
			return fmt.Errorf("%w: a pattern is too complex to compile", errSchemaNotAdmitted)
		}
		if work += n; work > maxSchemaRegexWork {
			return fmt.Errorf("%w: the patterns add up to more than %d compiled instructions", errSchemaNotAdmitted, maxSchemaRegexWork)
		}
		return nil
	}

	var walk func(node any) error
	walk = func(node any) error {
		obj, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		id := reflect.ValueOf(obj).Pointer()
		if visited[id] {
			return nil
		}
		visited[id] = true
		if steps++; steps&0xff == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if p, ok := obj["pattern"].(string); ok {
			if err := pattern(p); err != nil {
				return err
			}
		}
		if r, ok := obj["$ref"].(string); ok && strings.HasPrefix(r, "#") {
			refs = append(refs, r)
		}
		if pp, ok := obj["patternProperties"].(map[string]any); ok {
			for k, v := range pp {
				if err := pattern(k); err != nil {
					return err
				}
				if err := walk(v); err != nil {
					return err
				}
			}
		}
		for _, k := range schemaKeys {
			if err := walk(obj[k]); err != nil {
				return err
			}
		}
		for _, k := range schemaListKeys {
			if list, ok := obj[k].([]any); ok {
				for _, e := range list {
					if err := walk(e); err != nil {
						return err
					}
				}
			}
		}
		for _, k := range schemaMapKeys {
			if m, ok := obj[k].(map[string]any); ok {
				for _, v := range m {
					if err := walk(v); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return err
	}
	// A local $ref makes its target a schema wherever it sits.
	for resolved := 0; len(refs) > 0 && resolved < 10_000; resolved++ {
		ref := refs[0]
		refs = refs[1:]
		if target, ok := resolveLocalPointer(root, ref); ok {
			if err := walk(target); err != nil {
				return err
			}
		}
	}
	return nil
}

func resolveLocalPointer(root any, ref string) (any, bool) {
	if ref == "#" {
		return root, true
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	node := root
	for _, part := range strings.Split(ref[2:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch n := node.(type) {
		case map[string]any:
			child, ok := n[part]
			if !ok {
				return nil, false
			}
			node = child
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(n) {
				return nil, false
			}
			node = n[i]
		default:
			return nil, false
		}
	}
	return node, true
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
