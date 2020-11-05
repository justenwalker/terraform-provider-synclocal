---
layout: ""
page_title: "Resource: URL"
description: |-
    Sync a file from a url to a local destination
---

# Resource: URL

This resource syncs a file from a URL to a local destination.

~> This resource does not support update. Any change will result in a re-download.

!> This resource uses `If-Modified-Since` and `If-None-Match` headers to prevent downloading the same
file every time even if there were no changes. If the server does not support this, then the file will be downloaded
again on every run.

## Example Usage

```terraform
resource "synclocal_url" "webpage" {
  url = "http://www.example.org"
  destination = "/path/to/destination.html"
  headers = {
    "Authorization" = "Bearer AUTH_TOKEN"
  }
}
```

## Schema

### Required

- **filename** (String, Required) Destination file path
- **url** (String, Required) source url

### Optional

- **file_mode** (String, Optional) File mode for the destination (Octal String). Mirrors the source file if not provided.
- **headers** (Map of String, Optional) additional headers to add to the request
- **id** (String, Optional) The ID of this resource.

### Read-only

- **content_sha256** (String, Read-only) SHA256 hash of the file contents
- **etag** (String, Read-only) the etag of the resource
- **last_modified** (String, Read-only) the last modified date when it was retrieved from the upstream url