package domain

import "time"

type SMSEncoding string

const (
	SMSEncodingGSM7 SMSEncoding = "gsm7"
	SMSEncodingUCS2 SMSEncoding = "ucs2"
)

type SMSStatus string

const (
	SMSStatusReceivedUnread SMSStatus = "REC UNREAD"
	SMSStatusReceivedRead   SMSStatus = "REC READ"
	SMSStatusStoredUnsent   SMSStatus = "STO UNSENT"
	SMSStatusStoredSent     SMSStatus = "STO SENT"
	SMSStatusAll            SMSStatus = "ALL"
)

type SMSNotificationType string

const (
	SMSNotificationStored         SMSNotificationType = "stored"
	SMSNotificationDirect         SMSNotificationType = "direct"
	SMSNotificationDeliveryReport SMSNotificationType = "delivery_report"
)

type SMSMessage struct {
	Index     int         `json:"index"`
	Status    SMSStatus   `json:"status"`
	Address   string      `json:"address"`
	Timestamp string      `json:"timestamp,omitempty"`
	Body      string      `json:"body"`
	Encoding  SMSEncoding `json:"encoding,omitempty"`
}

type SMSSendRequest struct {
	To             string `json:"to"`
	Body           string `json:"body"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type SMSSendResult struct {
	MessageRefs []int       `json:"message_refs"`
	Encoding    SMSEncoding `json:"encoding"`
	Segments    int         `json:"segments"`
	SubmittedAt time.Time   `json:"submitted_at"`
}

type SMSNotification struct {
	Type       SMSNotificationType `json:"type"`
	Storage    string              `json:"storage,omitempty"`
	Index      int                 `json:"index,omitempty"`
	MessageRef int                 `json:"message_ref,omitempty"`
	Raw        string              `json:"raw,omitempty"`
	ReceivedAt time.Time           `json:"received_at"`
}
