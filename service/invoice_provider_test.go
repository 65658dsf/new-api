package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeInvoiceProviderTestJSON(t *testing.T, writer http.ResponseWriter, statusCode int, value any) {
	t.Helper()
	body, err := common.Marshal(value)
	require.NoError(t, err)
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(statusCode)
	_, err = writer.Write(body)
	require.NoError(t, err)
}

func newInvoiceProviderTestClient(t *testing.T, handler http.Handler, clientID string, clientSecret string) (*InvoiceProviderClient, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := NewInvoiceProviderClientWithBaseURL(server.URL, server.Client(), clientID, clientSecret)
	require.NoError(t, err)
	return client, server
}

func TestInvoiceProviderValidateOrdersUsesClientCredentialsAndCachesToken(t *testing.T) {
	var tokenCalls atomic.Int32
	var validateCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
		tokenCalls.Add(1)
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "application/x-www-form-urlencoded", request.Header.Get("Content-Type"))
		require.NoError(t, request.ParseForm())
		assert.Equal(t, "client_credentials", request.Form.Get("grant_type"))
		assert.Equal(t, "app-id", request.Form.Get("client_id"))
		assert.Equal(t, "app-secret", request.Form.Get("client_secret"))
		assert.Equal(t, invoiceProviderScope, request.Form.Get("scope"))
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"access_token": "cached-token",
			"token_type":   "Bearer",
			"expires_in":   3600,
			"scope":        invoiceProviderScope,
		})
	})
	mux.HandleFunc("/api/v1/invoice-orders/validate", func(writer http.ResponseWriter, request *http.Request) {
		validateCalls.Add(1)
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "Bearer cached-token", request.Header.Get("Authorization"))
		assert.Equal(t, "application/json", request.Header.Get("Content-Type"))
		var payload invoiceProviderValidateRequest
		require.NoError(t, common.DecodeJson(request.Body, &payload))
		assert.Equal(t, []string{"ORDER-1", "ORDER-2"}, payload.OrderNos)
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"code":      0,
			"message":   "ok",
			"requestId": "request-validate",
			"data": map[string]any{
				"orders": []map[string]any{
					{"orderNo": "ORDER-1", "goodsName": "Top up 1", "amount": "5.00", "currency": "CNY", "paidAt": "2026-07-22T10:00:00Z"},
					{"orderNo": "ORDER-2", "productName": "Top up 2", "amount": "7.00", "currency": "CNY", "paidAt": "2026-07-22T10:01:00Z"},
				},
				"totalAmount": "12.00",
				"currency":    "CNY",
			},
		})
	})
	client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")

	for range 2 {
		validation, err := client.ValidateOrders(context.Background(), []string{"ORDER-1", "ORDER-2"})
		require.NoError(t, err)
		require.Len(t, validation.Orders, 2)
		assert.Equal(t, "ORDER-1", validation.Orders[0].OrderNo)
		assert.Equal(t, "Top up 1", validation.Orders[0].GoodsName)
		assert.Equal(t, "Top up 2", validation.Orders[1].ProductName)
		assert.Equal(t, "12.00", validation.TotalAmount)
		assert.Equal(t, "CNY", validation.Currency)
	}

	assert.EqualValues(t, 1, tokenCalls.Load())
	assert.EqualValues(t, 2, validateCalls.Load())
}

func TestInvoiceProviderCreateInvoiceSupportsNestedTokenResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"code": 0,
			"data": map[string]any{
				"access_token": "nested-token",
				"token_type":   "Bearer",
				"expires_in":   "3600",
			},
		})
	})
	mux.HandleFunc("/api/v1/invoices", func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "Bearer nested-token", request.Header.Get("Authorization"))
		var payload InvoiceProviderCreateRequest
		require.NoError(t, common.DecodeJson(request.Body, &payload))
		assert.Equal(t, InvoiceProviderCreateRequest{
			OrderNos:         []string{"ORDER-9"},
			BuyerType:        "company",
			Title:            "Example Technology Co Ltd",
			TaxpayerID:       "91350211M000100Y43",
			BuyerAddress:     "Example Road 1",
			BuyerPhone:       "010-12345678",
			BuyerBank:        "Example Bank",
			BuyerBankAccount: "6222000000000000",
			RecipientEmail:   "invoice@example.com",
		}, payload)
		writeInvoiceProviderTestJSON(t, writer, http.StatusCreated, map[string]any{
			"code": 0,
			"data": map[string]any{
				"id":               91,
				"orderNos":         []string{"ORDER-9"},
				"orders":           []map[string]any{{"orderNo": "ORDER-9", "amount": "12.50", "currency": "CNY"}},
				"title":            "Example Technology Co Ltd",
				"buyerType":        "company",
				"taxpayerId":       "91350211M000100Y43",
				"buyerAddress":     "Example Road 1",
				"buyerPhone":       "010-12345678",
				"buyerBank":        "Example Bank",
				"buyerBankAccount": "6222000000000000",
				"recipientEmail":   "invoice@example.com",
				"totalAmount":      "12.50",
				"currency":         "CNY",
				"status":           "pending",
				"reviewNote":       "",
				"hasPdf":           false,
				"createdAt":        "2026-07-22T10:00:00Z",
			},
		})
	})
	client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")
	request := InvoiceProviderCreateRequest{
		OrderNos:         []string{"ORDER-9"},
		BuyerType:        "company",
		Title:            "Example Technology Co Ltd",
		TaxpayerID:       "91350211M000100Y43",
		BuyerAddress:     "Example Road 1",
		BuyerPhone:       "010-12345678",
		BuyerBank:        "Example Bank",
		BuyerBankAccount: "6222000000000000",
		RecipientEmail:   "invoice@example.com",
	}

	invoice, err := client.CreateInvoice(context.Background(), request)
	require.NoError(t, err)
	assert.EqualValues(t, 91, invoice.ID)
	assert.Equal(t, request.OrderNos, invoice.OrderNos)
	require.Len(t, invoice.Orders, 1)
	assert.Equal(t, request.TaxpayerID, invoice.TaxpayerID)
	assert.Equal(t, request.BuyerBank, invoice.BuyerBank)
	assert.Equal(t, request.RecipientEmail, invoice.RecipientEmail)
	assert.Equal(t, "12.50", invoice.TotalAmount)
	assert.Equal(t, InvoiceProviderStatusPending, invoice.Status)
	assert.False(t, invoice.HasPDF)
}

func TestInvoiceProviderListInvoicesMapsRemoteStatusFields(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"access_token": "list-token",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/api/v1/invoices", func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "2", request.URL.Query().Get("page"))
		assert.Equal(t, "50", request.URL.Query().Get("pageSize"))
		assert.Equal(t, "Bearer list-token", request.Header.Get("Authorization"))
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"code": 0,
			"data": map[string]any{
				"items": []map[string]any{
					{
						"id":               101,
						"orderNos":         []string{"ORDER-101"},
						"orders":           []map[string]any{{"orderNo": "ORDER-101", "amount": "20.00", "currency": "CNY"}},
						"title":            "Alice",
						"buyerType":        "individual",
						"taxpayerId":       "110101199001011234",
						"buyerAddress":     "",
						"buyerPhone":       "13800000000",
						"buyerBank":        "",
						"buyerBankAccount": "",
						"recipientEmail":   "alice@example.com",
						"totalAmount":      "20.00",
						"currency":         "CNY",
						"status":           "completed",
						"reviewNote":       "issued",
						"hasPdf":           true,
					},
				},
				"total":    51,
				"page":     2,
				"pageSize": 50,
			},
		})
	})
	client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")

	list, err := client.ListInvoices(context.Background(), 2, 50)
	require.NoError(t, err)
	assert.EqualValues(t, 51, list.Total)
	assert.Equal(t, 2, list.Page)
	assert.Equal(t, 50, list.PageSize)
	require.Len(t, list.Items, 1)
	assert.Equal(t, "individual", list.Items[0].BuyerType)
	assert.Equal(t, "110101199001011234", list.Items[0].TaxpayerID)
	assert.Equal(t, InvoiceProviderStatusCompleted, list.Items[0].Status)
	assert.Equal(t, "issued", list.Items[0].ReviewNote)
	assert.True(t, list.Items[0].HasPDF)
}

func TestInvoiceProviderDownloadPDFReturnsStreamingResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"access_token": "pdf-token",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/api/v1/invoices/42/pdf", func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "Bearer pdf-token", request.Header.Get("Authorization"))
		assert.Equal(t, "application/pdf", request.Header.Get("Accept"))
		writer.Header().Set("Content-Type", "application/pdf")
		writer.Header().Set("Content-Disposition", `attachment; filename="invoice-42.pdf"`)
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte("%PDF-1.7\nstreamed"))
		require.NoError(t, err)
	})
	client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")

	response, err := client.DownloadPDF(context.Background(), 42)
	require.NoError(t, err)
	defer response.Body.Close()
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "application/pdf", response.Header.Get("Content-Type"))
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.Equal(t, "%PDF-1.7\nstreamed", string(body))
}

func TestInvoiceProviderDownloadPDFRejectsSuccessfulJSONError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"access_token": "pdf-token",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/api/v1/invoices/9/pdf", func(writer http.ResponseWriter, request *http.Request) {
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"code":      "INVOICE_NOT_COMPLETED",
			"message":   "invoice is not completed",
			"requestId": "request-pdf-json",
		})
	})
	client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")

	response, err := client.DownloadPDF(context.Background(), 9)
	require.Error(t, err)
	require.Nil(t, response)
	var providerError *InvoiceProviderError
	require.ErrorAs(t, err, &providerError)
	assert.Equal(t, http.StatusOK, providerError.StatusCode)
	assert.Equal(t, "INVOICE_NOT_COMPLETED", providerError.Code)
	assert.Equal(t, "invoice is not completed", providerError.Message)
	assert.Equal(t, "request-pdf-json", providerError.RequestID)
}

func TestInvoiceProviderDownloadPDFRejectsSuccessfulNonPDFResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"access_token": "pdf-token",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/api/v1/invoices/10/pdf", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusOK)
		_, err := io.WriteString(writer, "not a pdf")
		require.NoError(t, err)
	})
	client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")

	response, err := client.DownloadPDF(context.Background(), 10)
	require.Error(t, err)
	assert.Nil(t, response)
	assert.Contains(t, err.Error(), "non-PDF")
}

func TestInvoiceProviderRefreshesTokenInsideRefreshWindow(t *testing.T) {
	var tokenCalls atomic.Int32
	var authMu sync.Mutex
	var authorizations []string
	mux := http.NewServeMux()
	mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
		call := tokenCalls.Add(1)
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"access_token": "token-" + strconv.FormatInt(int64(call), 10),
			"expires_in":   120,
		})
	})
	mux.HandleFunc("/api/v1/invoice-orders/validate", func(writer http.ResponseWriter, request *http.Request) {
		authMu.Lock()
		authorizations = append(authorizations, request.Header.Get("Authorization"))
		authMu.Unlock()
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"code": 0,
			"data": map[string]any{"orders": []any{}, "totalAmount": "0.00", "currency": "CNY"},
		})
	})
	client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")
	currentTime := time.Date(2026, time.July, 22, 10, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return currentTime }

	_, err := client.ValidateOrders(context.Background(), []string{"ORDER-1"})
	require.NoError(t, err)
	currentTime = currentTime.Add(89 * time.Second)
	_, err = client.ValidateOrders(context.Background(), []string{"ORDER-1"})
	require.NoError(t, err)
	currentTime = currentTime.Add(2 * time.Second)
	_, err = client.ValidateOrders(context.Background(), []string{"ORDER-1"})
	require.NoError(t, err)

	assert.EqualValues(t, 2, tokenCalls.Load())
	authMu.Lock()
	assert.Equal(t, []string{"Bearer token-1", "Bearer token-1", "Bearer token-2"}, authorizations)
	authMu.Unlock()
}

func TestInvoiceProviderRefreshesRejectedAccessToken(t *testing.T) {
	t.Run("JSON request", func(t *testing.T) {
		var tokenCalls atomic.Int32
		var validateCalls atomic.Int32
		mux := http.NewServeMux()
		mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
			call := tokenCalls.Add(1)
			writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
				"access_token": "token-" + strconv.FormatInt(int64(call), 10),
				"expires_in":   3600,
			})
		})
		mux.HandleFunc("/api/v1/invoice-orders/validate", func(writer http.ResponseWriter, request *http.Request) {
			validateCalls.Add(1)
			if request.Header.Get("Authorization") == "Bearer token-1" {
				writeInvoiceProviderTestJSON(t, writer, http.StatusUnauthorized, map[string]any{
					"code":    "AUTH_UNAUTHORIZED",
					"message": "expired token",
				})
				return
			}
			assert.Equal(t, "Bearer token-2", request.Header.Get("Authorization"))
			writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
				"code": 0,
				"data": map[string]any{
					"orders":      []map[string]any{{"orderNo": "ORDER-1", "amount": "5.00", "currency": "CNY"}},
					"totalAmount": "5.00",
					"currency":    "CNY",
				},
			})
		})
		client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")

		validation, err := client.ValidateOrders(context.Background(), []string{"ORDER-1"})
		require.NoError(t, err)
		require.Len(t, validation.Orders, 1)
		assert.EqualValues(t, 2, tokenCalls.Load())
		assert.EqualValues(t, 2, validateCalls.Load())
	})

	t.Run("PDF request", func(t *testing.T) {
		var tokenCalls atomic.Int32
		var pdfCalls atomic.Int32
		mux := http.NewServeMux()
		mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
			call := tokenCalls.Add(1)
			writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
				"access_token": "pdf-token-" + strconv.FormatInt(int64(call), 10),
				"expires_in":   3600,
			})
		})
		mux.HandleFunc("/api/v1/invoices/42/pdf", func(writer http.ResponseWriter, request *http.Request) {
			pdfCalls.Add(1)
			if request.Header.Get("Authorization") == "Bearer pdf-token-1" {
				writeInvoiceProviderTestJSON(t, writer, http.StatusUnauthorized, map[string]any{
					"code":    "AUTH_UNAUTHORIZED",
					"message": "expired token",
				})
				return
			}
			assert.Equal(t, "Bearer pdf-token-2", request.Header.Get("Authorization"))
			writer.Header().Set("Content-Type", "application/pdf")
			_, err := writer.Write([]byte("%PDF-1.7\nrefreshed"))
			require.NoError(t, err)
		})
		client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")

		response, err := client.DownloadPDF(context.Background(), 42)
		require.NoError(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.Equal(t, "%PDF-1.7\nrefreshed", string(body))
		assert.EqualValues(t, 2, tokenCalls.Load())
		assert.EqualValues(t, 2, pdfCalls.Load())
	})
}

func TestInvoiceProviderErrorsDoNotExposeCredentials(t *testing.T) {
	const clientID = "sensitive-app-id"
	const clientSecret = "sensitive-app-secret"
	t.Run("token HTTP error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
			writeInvoiceProviderTestJSON(t, writer, http.StatusUnauthorized, map[string]any{
				"code":      "OAUTH_INVALID_CLIENT",
				"message":   "invalid " + clientID + " credential " + clientSecret,
				"requestId": "request-token",
			})
		})
		client, _ := newInvoiceProviderTestClient(t, mux, clientID, clientSecret)

		_, err := client.ValidateOrders(context.Background(), []string{"ORDER-1"})
		require.Error(t, err)
		var providerError *InvoiceProviderError
		require.ErrorAs(t, err, &providerError)
		assert.Equal(t, http.StatusUnauthorized, providerError.StatusCode)
		assert.Equal(t, "OAUTH_INVALID_CLIENT", providerError.Code)
		assert.Equal(t, "request-token", providerError.RequestID)
		assert.NotContains(t, err.Error(), clientID)
		assert.NotContains(t, err.Error(), clientSecret)
	})

	t.Run("business error", func(t *testing.T) {
		mux := http.NewServeMux()
		mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
			writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
				"access_token": "sensitive-access-token",
				"expires_in":   3600,
			})
		})
		mux.HandleFunc("/api/v1/invoice-orders/validate", func(writer http.ResponseWriter, request *http.Request) {
			writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
				"code":      "ORDER_OCCUPIED",
				"message":   clientSecret + " sensitive-access-token",
				"requestId": "request-business",
			})
		})
		client, _ := newInvoiceProviderTestClient(t, mux, clientID, clientSecret)

		_, err := client.ValidateOrders(context.Background(), []string{"ORDER-1"})
		require.Error(t, err)
		var providerError *InvoiceProviderError
		require.ErrorAs(t, err, &providerError)
		assert.Equal(t, "ORDER_OCCUPIED", providerError.Code)
		assert.Equal(t, "request-business", providerError.RequestID)
		assert.NotContains(t, err.Error(), clientSecret)
		assert.NotContains(t, err.Error(), "sensitive-access-token")
	})
}

func TestInvoiceProviderDownloadPDFParsesJSONError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"access_token": "pdf-token",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/api/v1/invoices/8/pdf", func(writer http.ResponseWriter, request *http.Request) {
		writeInvoiceProviderTestJSON(t, writer, http.StatusConflict, map[string]any{
			"code":      "INVOICE_NOT_COMPLETED",
			"message":   "invoice is not completed",
			"requestId": "request-pdf",
		})
	})
	client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")

	response, err := client.DownloadPDF(context.Background(), 8)
	require.Error(t, err)
	require.Nil(t, response)
	var providerError *InvoiceProviderError
	require.ErrorAs(t, err, &providerError)
	assert.Equal(t, http.StatusConflict, providerError.StatusCode)
	assert.Equal(t, "INVOICE_NOT_COMPLETED", providerError.Code)
	assert.Equal(t, "invoice is not completed", providerError.Message)
	assert.Equal(t, "request-pdf", providerError.RequestID)
}

func TestInvoiceProviderRejectsOversizedJSONResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"access_token": "large-token",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/api/v1/invoice-orders/validate", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(writer, `{"code":0,"data":{"padding":"`+strings.Repeat("x", int(invoiceProviderMaxJSONResponseBytes))+`"}}`)
		require.NoError(t, err)
	})
	client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")

	_, err := client.ValidateOrders(context.Background(), []string{"ORDER-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too large")
}

func TestInvoiceProviderValidatesArgumentsBeforeSendingRequests(t *testing.T) {
	client := &InvoiceProviderClient{}

	_, err := client.ValidateOrders(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "order numbers")

	_, err = client.ValidateOrders(context.Background(), []string{"ORDER-1", "ORDER-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unique")

	_, err = client.ListInvoices(context.Background(), 0, 20)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "page")

	_, err = client.ListInvoices(context.Background(), 1, 101)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "page size")

	_, err = client.DownloadPDF(context.Background(), 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invoice ID")
}

func TestInvoiceProviderAcceptsOrderNumberAtModelLengthLimit(t *testing.T) {
	const orderNumberLength = 255
	var validateCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/api/oauth/token", func(writer http.ResponseWriter, request *http.Request) {
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"access_token": "boundary-token",
			"expires_in":   3600,
		})
	})
	mux.HandleFunc("/api/v1/invoice-orders/validate", func(writer http.ResponseWriter, request *http.Request) {
		validateCalls.Add(1)
		var payload invoiceProviderValidateRequest
		require.NoError(t, common.DecodeJson(request.Body, &payload))
		require.Len(t, payload.OrderNos, 1)
		assert.Len(t, payload.OrderNos[0], orderNumberLength)
		writeInvoiceProviderTestJSON(t, writer, http.StatusOK, map[string]any{
			"code": 0,
			"data": map[string]any{
				"orders":      []map[string]any{{"orderNo": payload.OrderNos[0], "amount": "1.00", "currency": "CNY"}},
				"totalAmount": "1.00",
				"currency":    "CNY",
			},
		})
	})
	client, _ := newInvoiceProviderTestClient(t, mux, "app-id", "app-secret")

	orderNumber := strings.Repeat("x", orderNumberLength)
	validation, err := client.ValidateOrders(context.Background(), []string{orderNumber})
	require.NoError(t, err)
	require.Len(t, validation.Orders, 1)
	assert.Equal(t, orderNumber, validation.Orders[0].OrderNo)

	_, err = client.ValidateOrders(context.Background(), []string{orderNumber + "x"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "order number")
	assert.EqualValues(t, 1, validateCalls.Load())
}

func TestInvoiceProviderTransportPreservesContextErrors(t *testing.T) {
	client, err := NewInvoiceProviderClientWithBaseURL("https://example.com", &http.Client{
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			return nil, context.Canceled
		}),
	}, "app-id", "app-secret")
	require.NoError(t, err)

	_, err = client.ValidateOrders(context.Background(), []string{"ORDER-1"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
}

type roundTripFunc func(request *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
