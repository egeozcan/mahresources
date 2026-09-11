package application_context

import (
	"bytes"
	"compress/flate"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"mahresources/auth"
	"mahresources/deferredtoken"
	"mahresources/models"
	"mahresources/mrql"
)

const mrqlSnapshotLifetime = time.Hour
const maxMRQLSnapshotBytes = 1024 * 1024

// ErrInvalidMRQLSnapshot asks the caller to run the query again, rather than
// silently replacing the sample whose pages or edit targets they were viewing.
var ErrInvalidMRQLSnapshot = errors.New("invalid or expired MRQL result snapshot; run the query again")

type mrqlSnapshot struct {
	Expires             int64
	Binding             string
	OrderPacked         string               `json:",omitempty"`
	Order               []MRQLEntityIdentity `json:",omitempty"`
	EntityType          string
	Warnings            []string
	DefaultLimitApplied bool
	AppliedLimit        int
}

func parseSnapshotQuery(query string, params map[string]any) (*mrql.Query, error) {
	parsed, err := mrql.Parse(query)
	if err != nil {
		return nil, err
	}
	if err := mrql.BindParams(parsed, params); err != nil {
		return nil, err
	}
	if err := mrql.Validate(parsed); err != nil {
		return nil, err
	}
	for _, order := range parsed.OrderBy {
		if order.Random {
			return parsed, nil
		}
	}
	return nil, ErrInvalidMRQLSnapshot
}

func (ctx *MahresourcesContext) mrqlSnapshotKey() []byte {
	key := sha256.Sum256(append([]byte("mrql-result-snapshot-v1\x00"), ctx.DeferredSigningKey()...))
	return key[:]
}

func (ctx *MahresourcesContext) mrqlSnapshotBinding(reqCtx context.Context, query string, params map[string]any, parsed *mrql.Query) (string, error) {
	opts, deny, err := ctx.mrqlQueryTranslateOptions(parsed)
	if err != nil {
		return "", err
	}
	principal := ctx.principal
	if principal == nil {
		principal = auth.PrincipalFromContext(reqCtx)
	}
	if len(params) == 0 {
		params = nil
	}
	// json.Marshal sorts map keys, so equivalent parameter maps have one
	// binding regardless of JSON field order. Scope resolution is included so
	// even a name-based SCOPE resolving to a different group invalidates it.
	raw, err := json.Marshal([]any{query, params, principal, opts.ScopeGroupID, deny})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// IssueMRQLSnapshot seals only the bounded, ordered identities of a RANDOM()
// execution. No entity data is cached and no client-authored identity is trusted.
func (ctx *MahresourcesContext) IssueMRQLSnapshot(reqCtx context.Context, query string, params map[string]any, result *MRQLResult) (string, error) {
	parsed, err := parseSnapshotQuery(query, params)
	if err != nil {
		return "", err
	}
	binding, err := ctx.mrqlSnapshotBinding(reqCtx, query, params, parsed)
	if err != nil {
		return "", err
	}
	order := result.Order
	if len(order) == 0 {
		for _, r := range result.Resources {
			order = append(order, MRQLEntityIdentity{EntityType: "resource", ID: r.ID})
		}
		for _, n := range result.Notes {
			order = append(order, MRQLEntityIdentity{EntityType: "note", ID: n.ID})
		}
		for _, g := range result.Groups {
			order = append(order, MRQLEntityIdentity{EntityType: "group", ID: g.ID})
		}
	}
	if len(order) > MaxMRQLInteractiveLimit {
		return "", ErrInvalidMRQLSnapshot
	}
	snapshot := mrqlSnapshot{Expires: time.Now().Add(mrqlSnapshotLifetime).Unix(), Binding: binding, Order: order,
		EntityType: result.EntityType, Warnings: result.Warnings,
		DefaultLimitApplied: result.DefaultLimitApplied, AppliedLimit: result.AppliedLimit}
	// Compress identities before sealing: a 10,000-item sample must fit small
	// authenticated JSON request limits on every subsequent page click.
	identities, err := json.Marshal(snapshot.Order)
	if err != nil {
		return "", err
	}
	var compressed bytes.Buffer
	compressor, err := flate.NewWriter(&compressed, flate.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := compressor.Write(identities); err != nil {
		return "", err
	}
	if err := compressor.Close(); err != nil {
		return "", err
	}
	snapshot.OrderPacked = base64.RawStdEncoding.EncodeToString(compressed.Bytes())
	snapshot.Order = nil
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	token := deferredtoken.Seal(ctx.mrqlSnapshotKey(), "mrql-result", 0, string(raw))
	if token == "" {
		return "", fmt.Errorf("could not issue MRQL result snapshot")
	}
	return token, nil
}

// ResolveMRQLSnapshot preserves the sample and its order while checking every
// identity against the live query and principal scope. Deleted, re-parented or
// otherwise nonmatching entities disappear; replacements are never sampled.
// The returned records contain IDs only, ready for the shared page hydrator or
// Mass Edit's existing target locking and count handshake.
func (ctx *MahresourcesContext) ResolveMRQLSnapshot(reqCtx context.Context, query string, params map[string]any, token string) (*MRQLResult, error) {
	if len(token) == 0 || len(token) > maxMRQLSnapshotBytes {
		return nil, ErrInvalidMRQLSnapshot
	}
	parsed, err := parseSnapshotQuery(query, params)
	if err != nil {
		return nil, err
	}
	binding, err := ctx.mrqlSnapshotBinding(reqCtx, query, params, parsed)
	if err != nil {
		return nil, err
	}
	typ, id, body, ok := deferredtoken.Open(ctx.mrqlSnapshotKey(), token)
	var snapshot mrqlSnapshot
	if !ok || typ != "mrql-result" || id != 0 || json.Unmarshal([]byte(body), &snapshot) != nil ||
		snapshot.Expires <= time.Now().Unix() || snapshot.Binding != binding || len(snapshot.Order) > MaxMRQLInteractiveLimit {
		return nil, ErrInvalidMRQLSnapshot
	}
	if snapshot.OrderPacked != "" {
		compressed, err := base64.RawStdEncoding.DecodeString(snapshot.OrderPacked)
		if err != nil {
			return nil, ErrInvalidMRQLSnapshot
		}
		reader := flate.NewReader(bytes.NewReader(compressed))
		raw, err := io.ReadAll(io.LimitReader(reader, maxMRQLSnapshotBytes+1))
		reader.Close()
		if err != nil || len(raw) > maxMRQLSnapshotBytes || json.Unmarshal(raw, &snapshot.Order) != nil || len(snapshot.Order) > MaxMRQLInteractiveLimit {
			return nil, ErrInvalidMRQLSnapshot
		}
	}
	reqCtx, cancel := context.WithTimeout(reqCtx, ctx.mrqlQueryTimeout())
	defer cancel()
	opts, deny, err := ctx.mrqlQueryTranslateOptions(parsed)
	if err != nil {
		return nil, err
	}
	result := &MRQLResult{EntityType: snapshot.EntityType, Warnings: snapshot.Warnings,
		DefaultLimitApplied: snapshot.DefaultLimitApplied, AppliedLimit: snapshot.AppliedLimit}
	if deny {
		return result, nil
	}
	visible := map[MRQLEntityIdentity]bool{}
	for _, entityType := range crossEntityTypes {
		var ids []uint
		for _, identity := range snapshot.Order {
			if identity.EntityType == entityType.String() {
				ids = append(ids, identity.ID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		branch := *parsed
		branch.EntityType, branch.OrderBy, branch.Limit, branch.Offset = entityType, nil, -1, -1
		for _, chunk := range chunkUints(ids, massEditChunkSize) {
			db, err := mrql.TranslateWithOptions(&branch, ctx.db.WithContext(reqCtx), opts)
			if err != nil {
				return nil, err
			}
			table := entityType.String() + "s"
			var rows []struct{ ID uint }
			if err := ctx.executeMRQLFind(db.Select(table+".id").Where(table+".id IN ?", chunk), &rows, &branch, "snapshot identity check"); err != nil {
				return nil, err
			}
			for _, row := range rows {
				visible[MRQLEntityIdentity{EntityType: entityType.String(), ID: row.ID}] = true
			}
		}
	}
	for _, identity := range snapshot.Order {
		if !visible[identity] {
			continue
		}
		result.Order = append(result.Order, identity)
		switch identity.EntityType {
		case "resource":
			result.Resources = append(result.Resources, models.Resource{ID: identity.ID})
		case "note":
			result.Notes = append(result.Notes, models.Note{ID: identity.ID})
		case "group":
			result.Groups = append(result.Groups, models.Group{ID: identity.ID})
		}
	}
	return result, nil
}
