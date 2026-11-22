package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/versatile-sh/connector-engine/port"
)

// ProblemType is the prefix of the type URN of every problem document.
const ProblemType = "urn:connectorengine:problem:"

// ProblemMediaType is the media type of a problem document (RFC 9457).
const ProblemMediaType = "application/problem+json"

// Statuses maps every error code the engine raises to its HTTP status; a code outside the map is an internal error.
var Statuses = map[port.Code]int{
	port.CodeInvalidArgument:            http.StatusBadRequest,
	port.CodeInvalidManifest:            http.StatusBadRequest,
	port.CodeInvalidPath:                http.StatusBadRequest,
	port.CodeNameTooLong:                http.StatusBadRequest,
	port.CodeInvalidAlias:               http.StatusBadRequest,
	port.CodeRowRefused:                 http.StatusBadRequest,
	port.CodePolicyRefused:              http.StatusBadRequest,
	port.CodeTokenInArguments:           http.StatusBadRequest,
	port.CodeUnauthenticated:            http.StatusUnauthorized,
	port.CodePermissionDenied:           http.StatusForbidden,
	port.CodeAgentPrincipalRefused:      http.StatusForbidden,
	port.CodeNotAnApprover:              http.StatusForbidden,
	port.CodeNotTheBoundPerson:          http.StatusForbidden,
	port.CodeNotFound:                   http.StatusNotFound,
	port.CodeTenantRemoved:              http.StatusGone,
	port.CodeConflict:                   http.StatusConflict,
	port.CodeInvalidTransition:          http.StatusConflict,
	port.CodeStaleApproval:              http.StatusConflict,
	port.CodeApprovalExpired:            http.StatusConflict,
	port.CodeDefinitionChanged:          http.StatusConflict,
	port.CodeClassChanged:               http.StatusConflict,
	port.CodeGrantRevoked:               http.StatusConflict,
	port.CodeGrantExhausted:             http.StatusConflict,
	port.CodeListingDeprecated:          http.StatusConflict,
	port.CodeCredentialRequired:         http.StatusConflict,
	port.CodeClientRegistrationRequired: http.StatusConflict,
	port.CodeIsolationUnmet:             http.StatusConflict,
	port.CodeNoApprover:                 http.StatusConflict,
	port.CodeTooManyPending:             http.StatusTooManyRequests,
	port.CodeRevisionMismatch:           http.StatusPreconditionFailed,
	port.CodeUnavailable:                http.StatusServiceUnavailable,
	port.CodeInternal:                   http.StatusInternalServerError,
}

// Problem is an RFC 9457 problem document.
type Problem struct {
	Status int
	Code   port.Code
	Detail string
}

// ProblemOf translates any error; internal detail never leaves the process.
func ProblemOf(err error) Problem {
	var e *port.Error
	if !errors.As(err, &e) || Statuses[e.Code] == 0 || e.Code == port.CodeInternal {
		return Problem{Status: http.StatusInternalServerError, Code: port.CodeInternal, Detail: "internal error"}
	}
	return Problem{Status: Statuses[e.Code], Code: e.Code, Detail: e.Detail}
}

// MarshalJSON writes type, title, status, detail and code.
func (p Problem) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"type": ProblemType + string(p.Code), "title": http.StatusText(p.Status), "status": p.Status, "detail": p.Detail, "code": p.Code})
}
