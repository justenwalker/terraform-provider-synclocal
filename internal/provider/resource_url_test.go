package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"
)

func TestAccResourceURL(t *testing.T) {
	file1 := testURLHandler(t, "./testdata/source-file01")
	file2 := testURLHandler(t, "./testdata/source-file02")
	srv := httptest.NewServer(&testResourceURLHandlers{
		Handlers: []http.Handler{
			file1, file1,
			file2, file2,
			file1,
			file1,
		},
	})
	httptest.NewRecorder()
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccDestroyURL,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
provider "synclocal" {
}

resource "synclocal_url" "copy" {
	url         = "%s"
	headers     = {
		Authorization = "Bearer secret"
	}
	filename = "./testdata/dest-file-url"
}
`, srv.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("synclocal_url.copy", "etag"),
					resource.TestCheckResourceAttr("synclocal_url.copy", "content_sha256", "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"),
				),
			},
			{
				Config: fmt.Sprintf(`
provider "synclocal" {
}

resource "synclocal_url" "copy" {
	url         = "%s"
	headers     = {
		Authorization = "Bearer secret"
	}
	filename = "./testdata/dest-file-url"
}
`, srv.URL),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("synclocal_url.copy", "etag"),
					resource.TestCheckResourceAttr("synclocal_url.copy", "content_sha256", "82e35a63ceba37e9646434c5dd412ea577147f1e4a41ccde1614253187e3dbf9"),
				),
			},
			{
				Config: fmt.Sprintf(`
provider "synclocal" {
}

resource "synclocal_url" "copy" {
	url         = "%s"
	filename = "./testdata/dest-file-url"
}
`, srv.URL),
				ExpectError: regexp.MustCompile(`.*not authorized.*`),
			},
			{
				Config: fmt.Sprintf(`
provider "synclocal" {
}

resource "synclocal_url" "copy" {
	url         = "%s"
	headers     = {
		Authorization = "Bearer bad"
	}
	filename = "./testdata/dest-file-url"
}
`, srv.URL),
				ExpectError: regexp.MustCompile(`.*forbidden.*`),
			},
		},
	})
}

func testAccDestroyURL(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "file" {
			continue
		}

		return os.Remove(rs.Primary.ID)
	}

	return nil
}

type testResourceURLHandlers struct {
	Handlers []http.Handler
	Index    int
}

func (h *testResourceURLHandlers) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := h.Handlers[h.Index]
	h.Index = (h.Index + 1) % len(h.Handlers)
	handler.ServeHTTP(w, r)
}

func testURLErrorHandler(status int, contentType string, content []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		w.Write(content)
	}
}

func testURLHandler(t *testing.T, filename string) http.HandlerFunc {
	content, hash := readTestFile(t, filename)
	modified := time.Now().UTC()
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("not authorized"))
			return
		}
		if auth != "Bearer secret" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("forbidden"))
			return
		}
		etag := r.Header.Get("If-None-Match")
		var ifModifiedSince time.Time
		if ims := r.Header.Get("If-Modified-Since"); ims != "" {
			ifModifiedSince, _ = time.Parse(time.RFC1123, ims)
		}
		if etag != "" {
			if etag == hash {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		} else if !ifModifiedSince.IsZero() {
			if ifModifiedSince.Before(modified) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		w.Header().Set("ETag", hash)
		w.Header().Set("Last-Modified", modified.Format(time.RFC1123))
		w.WriteHeader(http.StatusOK)
		if r.Method == http.MethodHead {
			return
		}
		w.Write(content)
	}
}

func readTestFile(t *testing.T, filename string) (content []byte, hash string) {
	fd, err := os.Open(filename)
	if err != nil {
		t.Fatalf("could not open %q: %v", filename, err)
	}
	defer fd.Close()
	h := sha256.New()
	tr := io.TeeReader(fd, h)
	data, err := ioutil.ReadAll(tr)
	if err != nil {
		t.Fatalf("could not read %q: %v", filename, err)
	}
	hstr := hex.EncodeToString(h.Sum(nil))
	return data, strconv.Quote(hstr)
}
