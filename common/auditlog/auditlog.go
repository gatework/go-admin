// Package auditlog is the contract between the request middleware that
// publishes audit records and the consumer that stores them: the queue topics
// and the status values written to sys_opera_log.status.
//
// It lives in common so that the producer (common/middleware) does not depend
// on the business module that consumes the records.
package auditlog

// Queue topics.
const (
	// LoginTopic carries one sys_login_log record per login or logout.
	LoginTopic = "login_log_queue"
	// OperationTopic carries one sys_opera_log record per audited request.
	OperationTopic = "operate_log_queue"
)

// Values of sys_opera_log.status.
const (
	StatusSucceeded = "1"
	StatusFailed    = "2"
)
