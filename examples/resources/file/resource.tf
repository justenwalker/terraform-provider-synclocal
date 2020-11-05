resource "synclocal_file" "copy" {
  source = "/path/to/source.txt"
  destination = "/path/to/dest.txt"
  file_mode = "0644"
}