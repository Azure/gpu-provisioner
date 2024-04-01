package errors

const (
	// Azure Resource Errors

	// ResourceNotFound is a const used to reference if we are missing a resource from azure
	ResourceNotFound = "ResourceNotFound"

	OperationNotAllowed = "OperationNotAllowed"

	// Error search terms
	SubscriptionQuotaExceededTerm = "Submit a request for Quota increase"
	RegionalQuotaExceededTerm     = "exceeding approved Total Regional Cores quota"
)
