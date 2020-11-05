provider "synclocal" {}

resource "synclocal_file" "new" {
  source      = "./internal/provider/testdata/source-file01"
  destination = "./internal/provider/testdata/dest-file"
}

terraform {
  required_providers {
    synclocal = {
      versions = ["0.1"]
      source = "registry.terraform.io/justenwalker/synclocal"
    }
  }
}
