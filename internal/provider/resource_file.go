package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

func resourceFile() *schema.Resource {
	return &schema.Resource{
		ReadContext:   resourceFileRead,
		CreateContext: resourceFileCreate,
		UpdateContext: resourceFileUpdate,
		DeleteContext: resourceFileDelete,
		CustomizeDiff: func(ctx context.Context, diff *schema.ResourceDiff, m interface{}) error {
			destHash, err := hashFile(diff.Get("destination").(string))
			if os.IsNotExist(err) {
				return diff.SetNewComputed("content_sha256")
			}

			srcHash, err := hashFile(diff.Get("source").(string))
			if err != nil {
				return err
			}
			if destHash != srcHash {
				return diff.SetNewComputed("content_sha256")
			}
			return nil
		},
		Schema: resourceFileSchema(),
	}
}

func resourceFileSchema() map[string]*schema.Schema {
	return map[string]*schema.Schema{
		"source": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "source file path",
		},
		"destination": {
			Type:        schema.TypeString,
			Required:    true,
			Description: "Destination file path",
			ForceNew:    true,
		},
		"file_mode": {
			Type:        schema.TypeString,
			Optional:    true,
			Description: "File mode for the destination (Octal String). Mirrors the source file if not provided.",
		},
		"content_sha256": {
			Type:        schema.TypeString,
			Computed:    true,
			Description: "SHA256 hash of the file contents",
		},
	}
}

func resourceFileDelete(ctx context.Context, data *schema.ResourceData, m interface{}) diag.Diagnostics {
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

func resourceFileRead(ctx context.Context, data *schema.ResourceData, m interface{}) (diags diag.Diagnostics) {
	file, err := idToFile(data.Id())
	if err != nil {
		return diag.FromErr(err)
	}
	fileHash, err := hashFile(file)

	if os.IsNotExist(err) {
		data.SetId("")
		return nil
	}
	if err != nil {
		return diag.FromErr(err)
	}
	data.Set("content_sha256", fileHash)
	return nil
}

func resourceFileUpdate(ctx context.Context, data *schema.ResourceData, m interface{}) (diags diag.Diagnostics) {
	diags = ensureCopyFile(data)
	if diags.HasError() {
		return
	}
	return resourceFileRead(ctx, data, m)
}

func resourceFileCreate(ctx context.Context, data *schema.ResourceData, m interface{}) (diags diag.Diagnostics) {
	diags = ensureCopyFile(data)
	if diags.HasError() {
		return diags
	}
	id, err := fileToID(data.Get("destination").(string))
	if err != nil {
		return diag.FromErr(err)
	}
	data.SetId(id)
	return
}

func ensureFileMode(data *schema.ResourceData) (diags diag.Diagnostics) {
	source := data.Get("source").(string)
	dest := data.Get("destination").(string)
	destStat, err := os.Stat(dest)
	if err != nil {
		return diag.FromErr(fmt.Errorf("could not stat destination %q: %w", dest, err))
	}
	var mode os.FileMode
	if v, ok := data.GetOk("file_mode"); ok {
		m, err := strconv.ParseUint(v.(string), 8, 32)
		if err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "file_mode is not a valid octal number",
				Detail:   err.Error(),
			})
			return
		}
		mode = os.FileMode(m)
	} else {
		srcStat, err := os.Stat(source)
		if err != nil {
			return diag.FromErr(fmt.Errorf("could not stat source %q: %w", dest, err))
		}
		mode = srcStat.Mode()
	}
	if mode == destStat.Mode() {
		return
	}
	if err := os.Chmod(dest, mode); err != nil {
		return diag.FromErr(fmt.Errorf("failed to chmod %s %q: %w", mode, dest, err))
	}
	return nil
}

func ensureCopyFile(data *schema.ResourceData) (diags diag.Diagnostics) {
	source := data.Get("source").(string)
	dest := data.Get("destination").(string)
	var mode os.FileMode
	sourceHash, err := hashFile(source)
	if err != nil {
		return diag.FromErr(err)
	}
	destHash, err := hashFile(dest)
	if err == nil && destHash == sourceHash {
		return ensureFileMode(data)
	}
	if v, ok := data.GetOk("file_mode"); ok {
		m, err := strconv.ParseUint(v.(string), 8, 32)
		if err != nil {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "file_mode is not a valid octal number",
				Detail:   err.Error(),
			})
			return
		}
		mode = os.FileMode(m)
	}
	if err := copyFile(source, dest, mode); err != nil {
		return diag.FromErr(err)
	}
	data.Set("content_sha256", sourceHash)
	return
}

func copyFile(source, destination string, mode os.FileMode) (err error) {
	var src, dest *os.File
	src, err = os.Open(source)
	if err != nil {
		return fmt.Errorf("could not open source file %q: %w", source, err)
	}
	defer src.Close()
	if mode == 0 {
		stat, err := src.Stat()
		if err != nil {
			return fmt.Errorf("could not stat source file %q: %w", source, err)
		}
		mode = stat.Mode()
	}
	dest, err = os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return fmt.Errorf("could not create destination file %q: %w", destination, err)
	}
	defer func() {
		closeErr := dest.Close()
		if err == nil {
			err = closeErr
		}
	}()
	if _, err = io.Copy(dest, src); err != nil {
		// clean up dest
		_ = dest.Close()
		_ = os.Remove(destination)
		return fmt.Errorf("error copying %q => %q: %w", source, destination, err)
	}
	return nil
}

func idToFile(id string) (string, error) {
	u, err := url.Parse(id)
	if err != nil {
		return "", fmt.Errorf("invalid id format %q: %w", id, err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("invalid id scheme %q, should be 'file'", u.Scheme)
	}
	return filepath.Abs(filepath.FromSlash(u.Path))
}

func fileToID(file string) (string, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	return (&url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(abs),
	}).String(), nil
}

func hashFile(filename string) (string, error) {
	h := sha256.New()
	fd, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer fd.Close()
	if _, err := io.Copy(h, fd); err != nil {
		return "", fmt.Errorf("could not hash file %q: %w", filename, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
