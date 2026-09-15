package setting

import "github.com/QuantumNous/new-api/common"

// SanitizeUpstreamErrorEnabled hides upstream error text from downstream
// clients. Gateways forward failures verbatim today, so an upstream body —
// which routinely carries provider account identifiers, balance or quota
// figures, internal hostnames and URLs, request IDs of another system, and
// sometimes fragments of the prompt — reaches whoever called the API.
//
// While enabled, a relay error whose text came from upstream is replaced by a
// fixed sentence that keeps the request ID. What is preserved: the HTTP status
// code, the protocol error type, and the error code, so clients can still
// branch on `insufficient_quota`, `rate_limit_exceeded`, `content_policy_
// violation` and friends. Operators lose nothing: the real upstream text stays
// in the server log and in the admin-only part of the error log.
//
// Errors this service authored itself (invalid request, insufficient quota of
// our own wallet, channel selection failures, rate limiting) keep their
// message, because the client cannot act on those failures otherwise.
//
// Bootstrap default comes from SANITIZE_UPSTREAM_ERROR; the root setting of the
// same name overrides it at runtime. Off by default so no existing deployment
// sees its responses change on upgrade.
var SanitizeUpstreamErrorEnabled = false

func init() {
	SanitizeUpstreamErrorEnabled = common.GetEnvOrDefaultBool("SANITIZE_UPSTREAM_ERROR", false)
}
