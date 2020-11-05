---
layout: ""
page_title: "Resource: File"
description: |-
    Sync a file from one location to another
---

# Resource: File

This resource syncs a local file from one place to another.

~> This resource does not support update. Any change will result in a re-copy

## Example Usage

```terraform
resource "synclocal_file" "copy" {
  source = "/path/to/source.txt"
  destination = "/path/to/dest.txt"
  file_mode = "0644"
}
```

## Schema

### Required

- **destination** (String, Required) Destination file path
- **source** (String, Required) source file path

### Optional

- **file_mode** (String, Optional) File mode for the destination (Octal String). Mirrors the source file if not provided.
- **id** (String, Optional) The ID of this resource.

### Read-only

- **content_sha256** (String, Read-only) SHA256 hash of the file contents