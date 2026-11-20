package port

import (
	"errors"
	"fmt"
)

// Code is a stable English error or reason code shared by every surface.
type Code string

// Generic codes of every surface.
const (
	CodeInvalidArgument  Code = "invalid_argument"
	CodeNotFound         Code = "not_found"
	CodePermissionDenied Code = "permission_denied"
	CodeUnauthenticated  Code = "unauthenticated"
	CodeConflict         Code = "conflict"
	CodeRevisionMismatch Code = "revision_mismatch"
	CodeUnavailable      Code = "unavailable"
	CodeInternal         Code = "internal"
	CodeUnknownTool      Code = "unknown_tool"
)

// Reasons a step refuses a call.
const (
	ReasonNoConnector        Code = "no_connector"
	ReasonConnectorDisabled  Code = "connector_disabled"
	ReasonNoBinding          Code = "no_binding"
	ReasonSessionEnded       Code = "session_ended"
	ReasonPersonRemoved      Code = "person_removed"
	ReasonNotInCatalog       Code = "not_in_catalog"
	ReasonListingQuarantined Code = "listing_quarantined"
	ReasonUnknownFunction    Code = "unknown_function"
	ReasonQuarantined        Code = "quarantined"
	ReasonRemoved            Code = "removed"
	ReasonDisabled           Code = "disabled"
	ReasonAppOnly            Code = "app_only"
	ReasonModelOnly          Code = "model_only"
	ReasonPaused             Code = "paused"
	ReasonNoWorkspaceGrant   Code = "no_workspace_grant"
	ReasonNoAgentGrant       Code = "no_agent_grant"
	ReasonDeniedByRow        Code = "denied_by_row"
	ReasonGrantStale         Code = "grant_stale"
	ReasonOverCatalogCeiling Code = "over_catalog_ceiling"
	ReasonExternalDeny       Code = "external_deny"
)

// Reasons that change a mode and refuse nothing.
const (
	NoteCappedByCatalogCeiling Code = "capped_by_catalog_ceiling"
	NoteCappedByDestructiveMax Code = "capped_by_destructive_max"
	NoteForcedAsk              Code = "forced_ask"
	NoteSessionGrant           Code = "session_grant"
	NoteExternalNarrowed       Code = "external_narrowed"
)

// Other codes and of the approval table.
const (
	CodeNoApprover            Code = "no_approver"
	CodeTooManyPending        Code = "too_many_pending"
	CodeApprovalExpired       Code = "approval_expired"
	CodeStaleApproval         Code = "stale_approval"
	CodeDefinitionChanged     Code = "definition_changed"
	CodeGrantRevoked          Code = "grant_revoked"
	CodeGrantExhausted        Code = "grant_exhausted"
	CodeCredentialRequired    Code = "credential_required"
	CodeNotAnApprover         Code = "not_an_approver"
	CodeIsolationUnmet        Code = "isolation_unmet"
	CodeListingDeprecated     Code = "listing_deprecated"
	CodeTokenInArguments      Code = "token_in_arguments"
	CodeNotTheBoundPerson     Code = "not_the_bound_person"
	CodeAgentPrincipalRefused Code = "agent_principal_refused"
	CodeTenantRemoved         Code = "tenant_removed"
	CodeRowRefused            Code = "row_refused"
	CodePolicyRefused         Code = "policy_subject_refused"
	CodeInvalidManifest       Code = "invalid_manifest"
	CodeInvalidPath           Code = "invalid_path"
	CodeNameTooLong           Code = "name_too_long"
	CodeInvalidAlias          Code = "invalid_alias"
	CodeInvalidTransition     Code = "invalid_transition"
	CodeClassChanged          Code = "class_changed"
	CodeOutcomeUnknown        Code = "outcome_unknown"
)

// Error is an error with a code the surfaces translate.
type Error struct {
	Code   Code
	Detail string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Detail) }

// Rejection is what an EventSink returns for an event its subscriber refuses for good; the relay gives that event up and goes on.
type Rejection struct {
	Event  string
	Detail string
}

func (r *Rejection) Error() string { return "event " + r.Event + " rejected: " + r.Detail }

// Errorf builds a coded error.
func Errorf(code Code, format string, args ...any) error {
	return &Error{Code: code, Detail: fmt.Sprintf(format, args...)}
}

// CodeOf returns the code of err, or internal when it carries none.
func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternal
}

// IsCode reports whether err carries the code.
func IsCode(err error, code Code) bool { return err != nil && CodeOf(err) == code }
