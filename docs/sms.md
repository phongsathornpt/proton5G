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

The `Idempotency-Key` request header can be used instead of the JSON field. Successful keys are cached for 10 minutes so retrying the same request does not transmit the message twice. Sending is limited to 10 accepted submissions per minute per daemon instance.

## Encoding

ASCII printable text uses the modem GSM text charset. Messages containing Thai or other Unicode are switched to UCS2 and encoded as UTF-16BE hexadecimal for the modem.

The UI and API report the selected encoding and number of submitted segments. Single-message limits are 160 characters for the basic GSM path and 70 Unicode code points for UCS2; longer input is split conservatively to 153/67 code points per submitted part.

> Note: the current implementation submits long parts individually in text mode. Some networks/handsets may display these as separate messages because text-mode `CMGS` does not provide an explicit concatenation UDH. True concatenated PDU-mode SMS should be used if strict reassembly is required.

## Serial behavior

`AT+CMGS` is interactive: the manager waits up to 10 seconds for the `>` prompt, writes the payload followed by Ctrl-Z, then waits up to 60 seconds for `+CMGS`/`OK` or `+CMS ERROR`.

Normal polling no longer resets the serial input buffer before every AT command. SMS URCs such as `+CMTI`, `+CMT`, and `+CDS` are removed from command responses and retained in a bounded in-memory queue so an inbound notification is not silently discarded between poll ticks.

## Troubleshooting

- `SMS send requires -token or FM350_API_TOKEN`: restart the daemon with API authentication configured.
- `+CMS ERROR`: the modem/network rejected the SMS. The numeric code is preserved in the error response.
- Empty inbox: confirm the SIM/modem storage contains messages and that the selected AT port supports SMS commands.
- Unicode appears incorrectly: confirm the modem firmware accepts `AT+CSCS="UCS2"` and hexadecimal UCS2 payloads.
