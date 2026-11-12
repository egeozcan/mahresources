package api_handlers

import (
	"net/http"
	"strconv"

	"mahresources/auth"
)

// generationRateKey names who a generation rate limit is charged to. With auth
// on that is the account, so rotating a forged source address cannot mint a
// fresh bucket and users behind one NAT do not starve each other. Otherwise
// (auth off, where the system principal has no user id) it is the client IP.
func generationRateKey(ctx interface{ ClientIP(*http.Request) string }, r *http.Request) string {
	if p := auth.PrincipalFromContext(r.Context()); p != nil && !p.SuperUser && p.UserID != 0 {
		return "user:" + strconv.FormatUint(uint64(p.UserID), 10)
	}
	return "ip:" + ctx.ClientIP(r)
}
