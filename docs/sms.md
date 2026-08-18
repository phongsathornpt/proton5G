# SMS support

The manager supports SMS over the FM350 AT port using the same exclusive AT gate as status polling and modem controls.

## WebUI

Open **SMS** in the sidebar to refresh stored messages, delete a stored message, or send a new SMS.

Sending and deleting are deliberately disabled unless the daemon is started with `-token` or `FM350_API_TOKEN`. SMS is billable and must not be exposed as an unauthenticated mutation endpoint.

## REST API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/sms` | List stored SMS messages (`AT+CMGL="ALL"`) |
| `GET` | `/api/sms/{index}` | Read one stored message (`AT+CMGR`) |
| `GET` | `/api/sms/notifications` | Drain queued `+CMTI`, `+CMT`, and `+CDS` notifications |
| `POST` | `/api/sms/send` | Send SMS (`AT+CMGS`, token required) |
| `DELETE` | `/api/sms/{index}` | Delete stored SMS (`AT+CMGD`, token required) |

Example send body:

```json
{
  "to": "+66812345678",
  "body": "hello",
  "idempotency_key": "optional-client-key"
}
```

The `Idempotency-Key` request header can be used instead of the JSON field. Successful keys are cached for 10 minutes so retrying the same request does not transmit the message twice. Sending is limited to 10 accepted submissions per minute per daemon instance and the API accepts at most 1000 message characters.

## Encoding and multipart messages

Short printable-ASCII messages use the modem GSM text charset. Short Thai/Unicode messages use UCS2 text mode and UTF-16BE hexadecimal data.

Messages that exceed one SMS are sent in **PDU mode as concatenated UCS2 SMS**. Each segment carries an 8-bit concatenation UDH (`00 03 <ref> <total> <sequence>`), so compatible handsets can reassemble the parts into one logical message. Multipart segmentation is based on UTF-16 code units, including correct handling for supplementary characters such as emoji.

The UI and API report the actual selected encoding and submitted segment count. The short-message limits are 160 printable ASCII characters or 70 UTF-16 code units for UCS2; concatenated PDU segments carry up to 67 UTF-16 code units each.

## Serial behavior and notifications

`AT+CMGS` is interactive: the manager waits up to 10 seconds for the `>` prompt, writes the payload followed by Ctrl-Z, then waits up to 60 seconds for `+CMGS`/`OK` or `+CMS ERROR`.

SMS setup requests `AT+CNMI=2,1,0,1,0` so stored-message notifications and delivery reports are enabled where firmware/network support them. Normal polling no longer resets the serial input buffer before every AT command. SMS URCs such as `+CMTI`, `+CMT`, and `+CDS` are removed from command responses and retained in a bounded in-memory queue instead of being silently discarded.

`GET /api/sms/notifications` drains this queue and returns typed notifications (`stored`, `direct`, or `delivery_report`) while preserving the raw modem URC for diagnostics.

## Troubleshooting

- `SMS send requires -token or FM350_API_TOKEN`: restart the daemon with API authentication configured.
- `+CMS ERROR`: the modem/network rejected the SMS. The numeric code is preserved in the error response.
- Empty inbox: confirm the SIM/modem storage contains messages and that the selected AT port supports SMS commands.
- Unicode appears incorrectly: confirm the modem firmware accepts `AT+CSCS="UCS2"` and hexadecimal UCS2 payloads.
- Missing delivery reports: carrier/SMSC support varies; inspect `/api/sms/notifications` for raw `+CDS` events.
