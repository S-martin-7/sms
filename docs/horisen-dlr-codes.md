# Horisen DLR Error Codes

Source: credentials workbook from Global Comunicaciones SPA (provider for the `SamuelOTP` HTTP connection), version 1.

These are the codes that arrive in the `errorCode` field of a DLR callback (our `POST /v1/horisen/dlr` handler). They are **not** the same as the submission response codes (100 / 101 / 102–116) that we handle in `internal/horisen/errors.go`.

## Error categories (informal grouping)

| Code | Description | Category |
|-----:|-------------|----------|
| 0    | No error                                          | Success |
| 1    | Unknown subscriber                                | Permanent: number doesn't exist |
| 9    | Illegal subscriber                                | Permanent |
| 11   | Teleservice not provisioned                       | Permanent |
| 13   | Call barred                                       | Permanent |
| 15   | CUG reject                                        | Permanent |
| 19   | No SMS support in MS                              | Permanent |
| 20   | Error in MS                                       | Permanent |
| 21   | Facility not supported                            | Permanent |
| 22   | Memory capacity exceeded                          | Temporary (handset full) |
| 29   | Absent subscriber                                 | Temporary (phone off/out of coverage) |
| 30   | MS busy for MT SMS                                | Temporary |
| 36   | Network/Protocol failure                          | Temporary |
| 44   | Illegal equipment                                 | Permanent |
| 60   | No paging response                                | Temporary |
| 61   | GMSC congestion                                   | Temporary |
| 63   | HLR timeout                                       | Temporary |
| 64   | MSC/SGSN timeout                                  | Temporary |
| 70   | SMRSE/TCP error                                   | Temporary |
| 72   | MT congestion                                     | Temporary |
| 75   | GPRS suspended                                    | Temporary |
| 80   | No paging response via MSC                        | Temporary |
| 81   | IMSI detached                                     | Temporary |
| 82   | Roaming restriction                               | Permanent |
| 83   | Deregistered in HLR for GSM                       | Permanent |
| 84   | Purged for GSM                                    | Permanent |
| 85   | No paging response via SGSN                       | Temporary |
| 86   | GPRS detached                                     | Temporary |
| 87   | Deregistered in HLR for GPRS                      | Permanent |
| 88   | The MS purged for GPRS                            | Permanent |
| 89   | Unidentified subscriber via MSC                   | Permanent |
| 90   | Unidentified subscriber via SGSN                  | Permanent |
| 112  | Originator missing credit on prepaid account      | Billing |
| 113  | Destination missing credit on prepaid account     | Billing |
| 114  | Error in prepaid system                           | Billing |
| 500  | Other error                                       | Unknown |
| 986  | Fixnet not allowed                                | Permanent |
| 987  | Message too long                                  | Permanent |
| 988  | MNP/System Error                                  | System |
| 989  | Supplier rejected SMS                             | System |
| 990  | HLR failure                                       | Temporary |
| 991  | Rejected by message text filter                   | Permanent (content) |
| 992  | Ported numbers not supported on destination       | Permanent |
| 993  | Blocklisted sender                                | Permanent |
| 994  | No credit                                         | Billing |
| 995  | Undeliverable                                     | Permanent |
| 996  | Validity expired                                  | Expired |
| 997  | Blocklisted receiver                              | Permanent |
| 998  | No route                                          | Permanent |
| 999  | Repeated submission (possible looping)            | System |

## Usage in our DLR handler

Plan task (future): `internal/horisen/dlrcodes.go` should expose:

```go
type DLRCategory string

const (
    DLRSuccess   DLRCategory = "success"
    DLRTemporary DLRCategory = "temporary" // worth reporting to tenant but don't block future sends
    DLRPermanent DLRCategory = "permanent" // tenant should probably flag this recipient
    DLRBilling   DLRCategory = "billing"   // operator-side: top up Horisen credit
    DLRSystem    DLRCategory = "system"    // provider-side; log for support ticket
    DLRUnknown   DLRCategory = "unknown"
)

func CategoryForDLRCode(code int) DLRCategory
func DescriptionForDLRCode(code int) string
```

Mapping lives in this doc; when we implement, mirror the table above.
