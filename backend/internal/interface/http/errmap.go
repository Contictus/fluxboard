package httpx

// The domain-error-to-HTTP mapping and JSON envelope moved to the response
// package (internal/interface/http/response) so handlers and middleware can
// share it without importing httpx (which would cycle: httpx -> handlers).
// See response.Error / response.JSON.
