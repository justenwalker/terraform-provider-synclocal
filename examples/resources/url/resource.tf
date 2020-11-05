resource "synclocal_url" "webpage" {
  url = "http://www.example.org"
  destination = "/path/to/destination.html"
  headers = {
    "Authorization" = "Bearer AUTH_TOKEN"
  }
}