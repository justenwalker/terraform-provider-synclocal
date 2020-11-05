package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func resourceURL() *schema.Resource {
	return &schema.Resource{
		ReadContext:   resourceURLRead,
		CreateContext: resourceURLCreate,
		DeleteContext: resourceURLDelete,
		CustomizeDiff: func(ctx context.Context, diff *schema.ResourceDiff, m interface{}) error {
			return nil
		},
		Schema: resourceURLSchema(),
	}
}

func resourceURLSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"url": {
			Type:        schema.TypeString,
			Required:    true,
			ForceNew:    true,
			Description: "source url",
		},
		"headers": {
			Type:        schema.TypeMap,
			Optional:    true,
			ForceNew:    true,
			Description: "additional headers to add to the request",
			Elem: &schema.Schema{
				Type: schema.TypeString,
			},
		},
		"filename": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "Destination file path",
			ForceNew:    true,
		},
		"file_mode": {
			Type:        schema.TypeString,
			Optional:    true,
			ForceNew:    true,
			Description: "File mode for the destination (Octal String). Mirrors the source file if not provided.",
		},
		"last_modified": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "the last modified date when it was retrieved from the upstream url",
		},
		"etag": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "the etag of the resource",
		},
		"content_sha256": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "SHA256 hash of the file contents",
		},
	}
}

func resourceURLDelete(ctx context.Context, data *schema.ResourceData, m interface{}) diag.Diagnostics {
	id := data.Id()
	name, err := idToFile(id)
	if err != nil {
		return diag.FromErr(err)
	}
	_, err = os.Stat(name)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return diag.FromErr(fmt.Errorf("could not stat file %q: %w", name, err))
	}
	if err := os.Remove(name); err != nil {
		return diag.FromErr(fmt.Errorf("could not remove file %q: %w", name, err))
	}
	return nil
}

func resourceURLRead(ctx context.Context, data *schema.ResourceData, m interface{}) (diags diag.Diagnostics) {
	file, err := idToFile(data.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	_, err = os.Stat(file)
	if os.IsNotExist(err) {
		data.SetId("")
		return nil
	}
	if err != nil {
		return diag.FromErr(err)
	}
	mode, err := getFileMode(data)
	if err != nil {
		return diag.FromErr(err)
	}
	return ensureDownloadFile(data, mode)
}

func resourceURLCreate(ctx context.Context, data *schema.ResourceData, m interface{}) (diags diag.Diagnostics) {
	mode, err := getFileMode(data)
	if err != nil {
		return diag.FromErr(err)
	}
	diags = ensureDownloadFile(data, mode)
	if diags.HasError() {
		return diags
	}
	id, err := fileToID(data.Get("filename").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	data.SetId(id)
	return
}

func makeRequest(method string, data *schema.ResourceData) (*http.Request, error) {
	source := data.Get("url").(string)
	var etag, modified string
	if v, ok := data.GetOk("etag"); ok {
		etag = v.(string)
	}
	if v, ok := data.GetOk("last_modified"); ok {
		modified = v.(string)
	}
	req, err := http.NewRequest(method, source, nil)
	if err != nil {
		return nil, err
	}
	if v, ok := data.GetOk("headers"); ok {
		m := v.(map[string]interface{})
		for k, v := range m {
			req.Header.Set(k, v.(string))
		}
	}
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	} else {
		if modified != "" {
			req.Header.Set("If-Modified-Since", modified)
		}
	}
	return req, nil
}

func getFileMode(data *schema.ResourceData) (os.FileMode, error) {
	if v, ok := data.GetOk("file_mode"); ok {
		m, err := strconv.ParseUint(v.(string), 8, 32)
		if err != nil {
			return 0, fmt.Errorf("file_mode is not a valid octal number")
		}
		return os.FileMode(m), nil
	}
	return os.FileMode(0664), nil
}

func ensureDownloadFile(data *schema.ResourceData, mode os.FileMode) (diags diag.Diagnostics) {
	req, err := makeRequest(http.MethodGet, data)
	if err != nil {
		return diag.FromErr(err)
	}
	c := &http.Client{}
	resp, err := c.Do(req)
	if err != nil {
		diag.FromErr(fmt.Errorf("error making request to %q: %w", req.URL, err))
	}

	dest := data.Get("filename").(string)

	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusNotModified:
		return diags
	case http.StatusOK:
		data.Set("etag", resp.Header.Get("ETag"))
		data.Set("last_modified", resp.Header.Get("Last-Modified"))
		h := sha256.New()
		tr := io.TeeReader(resp.Body, h)
		if err := writeResponseBody(tr, dest, mode); err != nil {
			return diag.FromErr(err)
		}
		shaStr := hex.EncodeToString(h.Sum(nil))
		data.Set("content_sha256", shaStr)
	case http.StatusUnauthorized:
		return diagResponseError(resp, "this url requires authorization. You may need to add Authorization header to this resource")
	case http.StatusForbidden:
		return diagResponseError(resp, "the server rejected your auth credentials. They may be expired or you may not be allowed to download this anymore.")
	default:
		return diagResponseError(resp, "the server returned an unexpected response code: %s", resp.Status)
	}
	return
}

func isTextual(contentType string) bool {
	mt := getNormalizedMediaType(contentType)
	if mt == "" {
		return false
	}
	if strings.HasPrefix(mt, "text/") {
		return true
	}
	// TODO: could use a map, not sure it matters at the moment
	switch mt {
	case "application/json", "application/xml", "application/yaml", "application/x-yaml":
		return true
	default:
		return false
	}
}

func getNormalizedMediaType(contentType string) string {
	// trim of the content-type parameters
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ""
	}
	n := len(mt)
	slash := strings.Index(mt, "/")
	if slash == -1 || slash == n-1 {
		// badly formed type like "application/"
		return ""
	}
	plus := strings.LastIndex(mt, "+")
	if plus == n-1 {
		// badly formed type like "application/something+"
		return ""
	}
	// normalize the media-type string
	// converts stuff like application/something+json to application/json
	if plus != -1 {
		return mt[:slash+1] + mt[plus+1:]
	}
	return mt
}

func diagResponseError(resp *http.Response, format string, v ...interface{}) (diags diag.Diagnostics) {
	var detail string
	if isTextual(resp.Header.Get("Content-Type")) {
		text, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Warning,
				Summary:  "could not read response body",
				Detail:   err.Error(),
			})
		} else {
			detail = string(text)
		}
	}
	diags = append(diags, diag.Diagnostic{
		Severity: diag.Error,
		Summary:  fmt.Sprintf(format, v...),
		Detail:   detail,
	})
	return
}

func writeResponseBody(body io.Reader, filename string, mode os.FileMode) (err error) {
	if mode == 0 {
		mode = os.FileMode(0644)
	}
	dest, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("could not create destination file %q: %w", filename, err)
	}
	defer func() {
		closeErr := dest.Close()
		if err == nil {
			err = closeErr
		}
	}()
	if _, err = io.Copy(dest, body); err != nil {
		// clean up dest
		_ = dest.Close()
		_ = os.Remove(filename)
		return fmt.Errorf("error reading request body into %q: %w", filename, err)
	}
	return nil
}
