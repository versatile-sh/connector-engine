// Package hints puts the class hints of a function into its raw definition, where listing resolution reads them.
package hints

// Key is the _meta key that carries the hints; they suggest a class and never set one.
const Key = "connector-engine/hints"

// Meta returns the _meta object of a definition carrying a gRPC idempotency level and an HTTP method, either possibly empty.
func Meta(grpcIdempotency, httpMethod string) map[string]any {
	return map[string]any{Key: map[string]string{"grpc_idempotency": grpcIdempotency, "http_method": httpMethod}}
}
