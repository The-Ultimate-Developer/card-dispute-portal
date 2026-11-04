package disputes

import (
	"slices"
	"time"
)

type Status string

const (
	StatusSubmitted   Status = "SUBMITTED"
	StatusUnderReview Status = "UNDER_REVIEW"
	StatusResolved    Status = "RESOLVED"
	StatusRejected    Status = "REJECTED"
)

// transitions is the whole dispute lifecycle in one place.
// Anything not listed here is rejected, including moves out of a final state.
var transitions = map[Status][]Status{
	StatusSubmitted:   {StatusUnderReview, StatusRejected},
	StatusUnderReview: {StatusResolved, StatusRejected},
}

func (s Status) Valid() bool {
	switch s {
	case StatusSubmitted, StatusUnderReview, StatusResolved, StatusRejected:
		return true
	}
	return false
}

func (s Status) CanTransitionTo(next Status) bool {
	return slices.Contains(transitions[s], next)
}

func (s Status) Final() bool {
	return s == StatusResolved || s == StatusRejected
}

type Reason string

const (
	ReasonUnrecognised          Reason = "UNRECOGNISED"
	ReasonDuplicate             Reason = "DUPLICATE"
	ReasonIncorrectAmount       Reason = "INCORRECT_AMOUNT"
	ReasonNotReceived           Reason = "NOT_RECEIVED"
	ReasonCancelledSubscription Reason = "CANCELLED_SUBSCRIPTION"
	ReasonOther                 Reason = "OTHER"
)

func (r Reason) Valid() bool {
	switch r {
	case ReasonUnrecognised, ReasonDuplicate, ReasonIncorrectAmount,
		ReasonNotReceived, ReasonCancelledSubscription, ReasonOther:
		return true
	}
	return false
}

type Dispute struct {
	ID                    string    `json:"id"`
	CustomerID            string    `json:"-"`
	Customer              *Customer `json:"customer,omitempty"`
	TransactionID         string    `json:"transactionId"`
	Merchant              string    `json:"merchant"`
	AmountCents           int64     `json:"amountCents"`
	Currency              string    `json:"currency"`
	CardLast4             string    `json:"cardLast4"`
	TransactionOccurredAt time.Time `json:"transactionOccurredAt"`
	Reason                Reason    `json:"reason"`
	Description           string    `json:"description"`
	Status                Status    `json:"status"`
	Version               int       `json:"version"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
}

// Customer is only attached for staff, who need to know whose dispute it is.
type Customer struct {
	ID       string `json:"id"`
	FullName string `json:"fullName"`
	Email    string `json:"email"`
}

type Event struct {
	ID         int64     `json:"id"`
	FromStatus *Status   `json:"fromStatus"`
	ToStatus   Status    `json:"toStatus"`
	ActorRole  string    `json:"actorRole"`
	Actor      *Actor    `json:"actor,omitempty"`
	Note       string    `json:"note"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Actor names the person behind a timeline event. Only staff see it: customers
// are told "Disputes team", never which employee handled their case.
type Actor struct {
	FullName string `json:"fullName"`
	Email    string `json:"email"`
}

type DisputeDetail struct {
	Dispute
	Timeline []Event `json:"timeline"`
}

type Summary struct {
	ID     string `json:"id"`
	Status Status `json:"status"`
}

type Page struct {
	Items      []Dispute `json:"items"`
	NextCursor string    `json:"nextCursor,omitempty"`
}
