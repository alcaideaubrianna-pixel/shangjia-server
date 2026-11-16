package domain

// SchedulerPool separates realtime work from history recovery.
type SchedulerPool string

const (
	PoolRealtime SchedulerPool = "realtime"
	PoolHistory  SchedulerPool = "history"
)

func (p SchedulerPool) Valid() bool { return p == PoolRealtime || p == PoolHistory }

// EventPriority controls delivery urgency.
type EventPriority int

const (
	PriorityNormal   EventPriority = 0
	PriorityRealtime EventPriority = 50
	PriorityUrgent   EventPriority = 100
)

func (p EventPriority) Valid() bool { return p >= PriorityNormal }

func (p EventPriority) IsRealtime() bool { return p >= PriorityRealtime }
func (p EventPriority) IsUrgent() bool   { return p >= PriorityUrgent }

// Route is the complete persisted routing identity for one delivery.
type Route struct {
	Pool      SchedulerPool
	Priority  EventPriority
	AccountID AccountID
	SourceID  SourceID
}

func (r Route) Valid() bool {
	return r.Pool.Valid() && r.Priority.Valid() && (r.AccountID.Valid() || r.SourceID.Valid())
}

func (r Route) AccountScoped() bool { return r.AccountID > 0 }
func (r Route) SourceScoped() bool  { return r.AccountID == 0 && r.SourceID > 0 }
func (r Route) History() bool       { return r.Pool == PoolHistory && !r.Priority.IsRealtime() }
func (r Route) Realtime() bool      { return !r.History() }
