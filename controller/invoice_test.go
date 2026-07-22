package controller

import (
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createInvoicePDFTestFile(t *testing.T, content string) *os.File {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "invoice-upload-*")
	require.NoError(t, err)
	_, err = file.WriteString(content)
	require.NoError(t, err)
	_, err = file.Seek(0, 0)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = file.Close()
	})
	return file
}

func TestSaveUploadedInvoicePDFAcceptsValidPDF(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("INVOICE_FILE_DIR", dir)

	content := "%PDF-1.7\ninvoice body"
	file := createInvoicePDFTestFile(t, content)
	header := &multipart.FileHeader{
		Filename: "issued-invoice.pdf",
		Size:     int64(len(content)),
	}

	storedName, err := saveUploadedInvoicePDF(42, header, file)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(storedName, "invoice-42-"))
	assert.True(t, strings.HasSuffix(storedName, ".pdf"))

	saved, err := os.ReadFile(filepath.Join(dir, storedName))
	require.NoError(t, err)
	assert.Equal(t, content, string(saved))
}

func TestSaveUploadedInvoicePDFRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		content string
		size    int64
		wantErr string
	}{
		{
			name:    "wrong extension",
			file:    "invoice.txt",
			content: "%PDF-1.7\nbody",
			size:    13,
			wantErr: "PDF 格式",
		},
		{
			name:    "wrong file header",
			file:    "invoice.pdf",
			content: "not a pdf",
			size:    9,
			wantErr: "有效的 PDF",
		},
		{
			name:    "too large",
			file:    "invoice.pdf",
			content: "%PDF-1.7\nbody",
			size:    invoicePDFMaxBytes + 1,
			wantErr: "10MB",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("INVOICE_FILE_DIR", t.TempDir())
			file := createInvoicePDFTestFile(t, tc.content)
			header := &multipart.FileHeader{
				Filename: tc.file,
				Size:     tc.size,
			}

			_, err := saveUploadedInvoicePDF(42, header, file)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestDeleteInvoicePDFRemovesStoredFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("INVOICE_FILE_DIR", dir)

	fileName := "invoice-delete-test.pdf"
	filePath := filepath.Join(dir, fileName)
	require.NoError(t, os.WriteFile(filePath, []byte("%PDF-1.7\nbody"), 0600))

	require.NoError(t, deleteInvoicePDF(fileName))
	_, err := os.Stat(filePath)
	require.True(t, os.IsNotExist(err))
	require.NoError(t, deleteInvoicePDF(fileName))
}

func TestInvoiceProviderOrderNosMatchRequiresSameLogicalOrders(t *testing.T) {
	tests := []struct {
		name      string
		invoice   *service.InvoiceProviderInvoice
		requested []string
		want      bool
	}{
		{
			name: "exact order number set",
			invoice: &service.InvoiceProviderInvoice{
				ID:       1,
				OrderNos: []string{"ORDER-2", "ORDER-1"},
			},
			requested: []string{"ORDER-1", "ORDER-2"},
			want:      true,
		},
		{
			name: "external order aliases",
			invoice: &service.InvoiceProviderInvoice{
				ID: 2,
				Orders: []service.InvoiceProviderOrder{
					{OrderNo: "REMOTE-1", ExternalOrderNo: "ORDER-1"},
					{OrderNo: "REMOTE-2", ExternalNo: "ORDER-2"},
				},
			},
			requested: []string{"ORDER-1", "ORDER-2"},
			want:      true,
		},
		{
			name: "remote invoice has an extra order",
			invoice: &service.InvoiceProviderInvoice{
				ID:       3,
				OrderNos: []string{"ORDER-1", "ORDER-2"},
			},
			requested: []string{"ORDER-1"},
			want:      false,
		},
		{
			name: "same count with a different order",
			invoice: &service.InvoiceProviderInvoice{
				ID:       5,
				OrderNos: []string{"ORDER-1", "ORDER-3"},
			},
			requested: []string{"ORDER-1", "ORDER-2"},
			want:      false,
		},
		{
			name: "two requested aliases identify one logical order",
			invoice: &service.InvoiceProviderInvoice{
				ID: 4,
				Orders: []service.InvoiceProviderOrder{
					{OrderNo: "ORDER-1", ExternalOrderNo: "EXTERNAL-1"},
					{OrderNo: "ORDER-2"},
				},
			},
			requested: []string{"ORDER-1", "EXTERNAL-1"},
			want:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			requested := make(map[string]struct{}, len(tc.requested))
			for _, orderNo := range tc.requested {
				requested[orderNo] = struct{}{}
			}
			assert.Equal(t, tc.want, invoiceProviderOrderNosMatch(tc.invoice, requested))
		})
	}
}
