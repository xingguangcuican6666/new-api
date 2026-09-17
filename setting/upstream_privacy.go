package setting

import "github.com/QuantumNous/new-api/common"

// UpstreamPrivacyProtectionEnabled hides upstream channel details — the model
// mapping result — from end users. Gateways normally record which upstream
// model actually served a mapped request (`is_model_mapped` /
// `upstream_model_name` in the consume log's other JSON, and
// `properties.upstream_model_name` on task records), and that mapping reveals
// which provider and upstream model sit behind a public model name.
//
// While enabled, those fields are stripped from every user-visible projection:
// self/token usage logs and the user's task list and task fetch responses.
// Administrators lose nothing: the dashboard "all logs" / "all tasks" views
// keep showing the mapping, and the stored log rows are untouched, so turning
// the toggle off restores user visibility immediately.
//
// Bootstrap default comes from UPSTREAM_PRIVACY_PROTECTION; the root setting
// of the same name overrides it at runtime. Off by default so no existing
// deployment hides information users previously saw.
var UpstreamPrivacyProtectionEnabled = false

func init() {
	UpstreamPrivacyProtectionEnabled = common.GetEnvOrDefaultBool("UPSTREAM_PRIVACY_PROTECTION", false)
}
