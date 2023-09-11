## smtp 
---


`smtp` implements bindings for `smtp` protocol in javascript
to be used from nuclei scanner.



## Types

### SMTPClient

 SMTPClient is a minimal SMTP client for nuclei scripts.

| Method | Description | Arguments | Returns |
|--------|-------------|-----------|---------|
| `IsSMTP` |  IsSMTP checks if a host is running a SMTP server. | `host`, `port` | `IsSMTPResponse`, `error` |




## Exported Types Fields
### IsSMTPResponse

| Name | Type | 
|--------|-------------|
| Banner | `string` |
| IsSMTP | `bool` |




