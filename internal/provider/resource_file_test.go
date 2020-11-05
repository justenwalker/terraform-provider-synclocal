package provider

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"os"
	"testing"
)

func TestAccResourceFile(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:          func() { testAccPreCheck(t) },
		ProviderFactories: testAccProviderFactories,
		CheckDestroy:      testAccDestroyFile,
		Steps: []resource.TestStep{
			{
				Config: `
provider "synclocal" {
}

resource "synclocal_file" "copy" {
	source      = "./testdata/source-file01"
	destination = "./testdata/dest-file"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("synclocal_file.copy", "content_sha256"),
				),
			},
			{
				Config: `
provider "synclocal" {
}

resource "synclocal_file" "copy" {
	source      = "./testdata/source-file01"
	destination = "./testdata/dest-file"
}
`,
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("synclocal_file.copy", "content_sha256"),
				),
			},
		},
	})
}

func testAccDestroyFile(s *terraform.State) error {
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "file" {
			continue
		}

		return os.Remove(rs.Primary.ID)
	}

	return nil
}
