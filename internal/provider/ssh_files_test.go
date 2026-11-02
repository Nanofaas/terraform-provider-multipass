package provider

import (
	"bytes"
	"context"
	"github.com/Nanofaas/terraform-provider-multipass/internal/multipasscli"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"os"
	"path/filepath"
	"testing"
)

func TestSSHRecursiveTransferRejectedBeforeMutation(t *testing.T) {
	upload := &fileUploadResource{sshMode: true, client: &testClient{}}
	d := upload.transferUpload(context.Background(), &fileUploadResourceModel{Recursive: types.BoolValue(true)})
	if !d.HasError() {
		t.Fatal("recursive upload allowed")
	}
	download := &fileDownloadResource{sshMode: true, client: &testClient{}}
	d = download.downloadAndWrite(context.Background(), &fileDownloadResourceModel{Recursive: types.BoolValue(true)})
	if !d.HasError() {
		t.Fatal("recursive download allowed")
	}
}
func TestSSHFileDownloadUsesLocalDestination(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "result")
	input := []byte{0, 255, 10, 13}
	r := &fileDownloadResource{sshMode: true, hostOS: "darwin", client: &testClient{transferCapture: func(_ context.Context, opts multipasscli.TransferOptions) ([]byte, error) {
		if opts.Destination != "-" {
			t.Fatalf("remote local destination: %s", opts.Destination)
		}
		return input, nil
	}}}
	d := r.downloadAndWrite(context.Background(), &fileDownloadResourceModel{Instance: types.StringValue("vm"), Source: types.StringValue("/file"), Destination: types.StringValue(dest)})
	if d.HasError() {
		t.Fatal(d)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, input) {
		t.Fatalf("bytes=%v err=%v", got, err)
	}
}
