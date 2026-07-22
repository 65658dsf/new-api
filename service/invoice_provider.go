package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	invoiceProviderBaseURL              = "https://oauth.xzncraft.cn"
	invoiceProviderScope                = "invoice.apply"
	invoiceProviderMaxJSONResponseBytes = int64(2 << 20)
	invoiceProviderTokenRefreshWindow   = 30 * time.Second
	invoiceProviderDefaultTokenTTL      = 5 * time.Minute
	invoiceProviderMaxErrorTextBytes    = 512
	invoiceProviderMaxOrderCount        = 20
	invoiceProviderMaxOrderNoBytes      = 255
	invoiceProviderPDFProbeBytes        = 512
)

const (
	InvoiceProviderStatusPending   = "pending"
	InvoiceProviderStatusApproved  = "approved"
	InvoiceProviderStatusRejected  = "rejected"
	InvoiceProviderStatusCompleted = "completed"
)

// InvoiceProviderClient calls the Xingzhining invoice API with an application
// access token. A client instance should be reused so its token cache is useful.
type InvoiceProviderClient struct {
	baseURL      string
	httpClient   *http.Client
	clientID     string
	clientSecret string

	tokenMu        sync.Mutex
	accessToken    string
	tokenExpiresAt time.Time
	now            func() time.Time
}

type InvoiceProviderOrder struct {
	OrderNo         string `json:"orderNo"`
	ExternalNo      string `json:"externalNo,omitempty"`
	ExternalOrderNo string `json:"externalOrderNo,omitempty"`
	GoodsName       string `json:"goodsName,omitempty"`
	ProductName     string `json:"productName,omitempty"`
	Amount          string `json:"amount"`
	Currency        string `json:"currency"`
	PaidAt          string `json:"paidAt,omitempty"`
}

type InvoiceProviderOrderValidation struct {
	Orders      []InvoiceProviderOrder `json:"orders"`
	TotalAmount string                 `json:"totalAmount"`
	Currency    string                 `json:"currency"`
}

type InvoiceProviderCreateRequest struct {
	OrderNos         []string `json:"orderNos"`
	BuyerType        string   `json:"buyerType"`
	Title            string   `json:"title"`
	TaxpayerID       string   `json:"taxpayerId"`
	BuyerAddress     string   `json:"buyerAddress,omitempty"`
	BuyerPhone       string   `json:"buyerPhone,omitempty"`
	BuyerBank        string   `json:"buyerBank,omitempty"`
	BuyerBankAccount string   `json:"buyerBankAccount,omitempty"`
	RecipientEmail   string   `json:"recipientEmail"`
}

type InvoiceProviderInvoice struct {
	ID               int64                  `json:"id"`
	OrderNos         []string               `json:"orderNos"`
	Orders           []InvoiceProviderOrder `json:"orders"`
	Title            string                 `json:"title"`
	BuyerType        string                 `json:"buyerType"`
	TaxpayerID       string                 `json:"taxpayerId"`
	BuyerAddress     string                 `json:"buyerAddress"`
	BuyerPhone       string                 `json:"buyerPhone"`
	BuyerBank        string                 `json:"buyerBank"`
	BuyerBankAccount string                 `json:"buyerBankAccount"`
	RecipientEmail   string                 `json:"recipientEmail"`
	TotalAmount      string                 `json:"totalAmount"`
	Currency         string                 `json:"currency"`
	Status           string                 `json:"status"`
	ReviewNote       string                 `json:"reviewNote"`
	HasPDF           bool                   `json:"hasPdf"`
	CreatedAt        any                    `json:"createdAt,omitempty"`
	UpdatedAt        any                    `json:"updatedAt,omitempty"`
}

type InvoiceProviderInvoiceList struct {
	Items    []InvoiceProviderInvoice `json:"items"`
	Total    int64                    `json:"total"`
	Page     int                      `json:"page"`
	PageSize int                      `json:"pageSize"`
}

// InvoiceProviderError contains the provider's stable error fields without
// retaining request bodies, authorization headers, or application credentials.
type InvoiceProviderError struct {
	StatusCode int
	Code       string
	Message    string
	RequestID  string
}

func (e *InvoiceProviderError) Error() string {
	if e == nil {
		return "invoice provider request failed"
	}
	parts := []string{"invoice provider request failed"}
	if e.StatusCode != 0 {
		parts = append(parts, "status="+strconv.Itoa(e.StatusCode))
	}
	if e.Code != "" {
		parts = append(parts, "code="+e.Code)
	}
	if e.Message != "" {
		parts = append(parts, "message="+e.Message)
	}
	if e.RequestID != "" {
		parts = append(parts, "request_id="+e.RequestID)
	}
	return strings.Join(parts, " ")
}

type invoiceProviderEnvelope[T any] struct {
	Code      any    `json:"code"`
	Message   string `json:"message"`
	Data      T      `json:"data"`
	RequestID string `json:"requestId"`
}

type invoiceProviderErrorEnvelope struct {
	Code             any    `json:"code"`
	Message          string `json:"message"`
	RequestID        string `json:"requestId"`
	OAuthError       string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type invoiceProviderTokenFields struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   any    `json:"expires_in"`
	Scope       string `json:"scope"`
}

type invoiceProviderTokenResponse struct {
	Code      any    `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
	invoiceProviderTokenFields
	Data invoiceProviderTokenFields `json:"data"`
}

type invoiceProviderValidateRequest struct {
	OrderNos []string `json:"orderNos"`
}

// NewInvoiceProviderClient creates a production client for the fixed provider
// origin. The general provider HTTP client is used when it has been initialized.
func NewInvoiceProviderClient(clientID string, clientSecret string) *InvoiceProviderClient {
	httpClient := GetHttpClient()
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &InvoiceProviderClient{
		baseURL:      invoiceProviderBaseURL,
		httpClient:   httpClient,
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
		now:          time.Now,
	}
}

// NewInvoiceProviderClientWithBaseURL permits an injected origin and HTTP
// client for deterministic tests. Production code should use
// NewInvoiceProviderClient so the upstream origin cannot come from user input.
func NewInvoiceProviderClientWithBaseURL(baseURL string, httpClient *http.Client, clientID string, clientSecret string) (*InvoiceProviderClient, error) {
	parsedURL, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.User != nil || parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		return nil, errors.New("invalid invoice provider base URL")
	}
	if httpClient == nil {
		return nil, errors.New("invoice provider HTTP client is required")
	}

	return &InvoiceProviderClient{
		baseURL:      strings.TrimRight(parsedURL.String(), "/"),
		httpClient:   httpClient,
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
		now:          time.Now,
	}, nil
}

func (c *InvoiceProviderClient) ValidateOrders(ctx context.Context, orderNos []string) (*InvoiceProviderOrderValidation, error) {
	if err := validateInvoiceProviderOrderNos(orderNos); err != nil {
		return nil, err
	}
	return invoiceProviderDoJSON[InvoiceProviderOrderValidation](c, ctx, http.MethodPost, "/api/v1/invoice-orders/validate", invoiceProviderValidateRequest{OrderNos: orderNos})
}

func (c *InvoiceProviderClient) CreateInvoice(ctx context.Context, request InvoiceProviderCreateRequest) (*InvoiceProviderInvoice, error) {
	if err := validateInvoiceProviderOrderNos(request.OrderNos); err != nil {
		return nil, err
	}
	return invoiceProviderDoJSON[InvoiceProviderInvoice](c, ctx, http.MethodPost, "/api/v1/invoices", request)
}

func (c *InvoiceProviderClient) ListInvoices(ctx context.Context, page int, pageSize int) (*InvoiceProviderInvoiceList, error) {
	if page < 1 {
		return nil, errors.New("invoice provider page must be at least 1")
	}
	if pageSize < 1 || pageSize > 100 {
		return nil, errors.New("invoice provider page size must be between 1 and 100")
	}
	path := "/api/v1/invoices?page=" + strconv.Itoa(page) + "&pageSize=" + strconv.Itoa(pageSize)
	return invoiceProviderDoJSON[InvoiceProviderInvoiceList](c, ctx, http.MethodGet, path, nil)
}

// DownloadPDF returns a successful upstream response without consuming its
// body so the controller can stream it directly. The caller owns Body.Close.
func (c *InvoiceProviderClient) DownloadPDF(ctx context.Context, invoiceID int64) (*http.Response, error) {
	if invoiceID <= 0 {
		return nil, errors.New("invoice provider invoice ID must be positive")
	}
	token, err := c.accessTokenForRequest(ctx)
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/invoices/"+strconv.FormatInt(invoiceID, 10)+"/pdf", nil)
		if err != nil {
			return nil, errors.New("failed to create invoice provider PDF request")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/pdf")

		response, err := c.httpClient.Do(req)
		if err != nil {
			return nil, c.transportError(err, token)
		}
		if response.StatusCode == http.StatusUnauthorized {
			c.invalidateAccessToken(token)
			if attempt == 0 {
				_ = response.Body.Close()
				token, err = c.accessTokenForRequest(ctx)
				if err != nil {
					return nil, err
				}
				continue
			}
		}
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			defer response.Body.Close()
			return nil, c.responseError(response, token)
		}

		if err := validateInvoiceProviderPDFResponse(response, c, token); err != nil {
			_ = response.Body.Close()
			return nil, err
		}
		return response, nil
	}
	return nil, errors.New("invoice provider PDF request failed")
}

// validateInvoiceProviderPDFResponse probes a successful response while
// preserving the bytes for the streaming caller. The provider normally uses
// application/pdf, but a business error may still be returned as a 2xx JSON
// envelope, so both the media type and the PDF magic header are checked.
func validateInvoiceProviderPDFResponse(response *http.Response, client *InvoiceProviderClient, token string) error {
	if response == nil || response.Body == nil {
		return errors.New("invoice provider PDF response has no body")
	}

	probe, err := io.ReadAll(io.LimitReader(response.Body, invoiceProviderPDFProbeBytes))
	if err != nil {
		return fmt.Errorf("failed to read invoice provider PDF response: %w", err)
	}
	response.Body = &invoiceProviderReadCloser{
		Reader: io.MultiReader(bytes.NewReader(probe), response.Body),
		Closer: response.Body,
	}

	contentType := strings.TrimSpace(response.Header.Get("Content-Type"))
	mediaType := contentType
	if parsed, _, parseErr := mime.ParseMediaType(contentType); parseErr == nil {
		mediaType = parsed
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	trimmedProbe := bytes.TrimSpace(probe)
	jsonResponse := strings.HasSuffix(mediaType, "+json") || mediaType == "application/json" || mediaType == "text/json"
	if len(trimmedProbe) > 0 && (trimmedProbe[0] == '{' || trimmedProbe[0] == '[') {
		jsonResponse = true
	}
	if jsonResponse {
		return client.responseError(response, token)
	}
	if !bytes.HasPrefix(probe, []byte("%PDF-")) {
		return errors.New("invoice provider returned a non-PDF response")
	}
	return nil
}

type invoiceProviderReadCloser struct {
	io.Reader
	io.Closer
}

func invoiceProviderDoJSON[T any](c *InvoiceProviderClient, ctx context.Context, method string, path string, requestBody any) (*T, error) {
	if c == nil || c.httpClient == nil || c.now == nil {
		return nil, errors.New("invoice provider client is not initialized")
	}
	token, err := c.accessTokenForRequest(ctx)
	if err != nil {
		return nil, err
	}

	var bodyBytes []byte
	if requestBody != nil {
		bodyBytes, err = common.Marshal(requestBody)
		if err != nil {
			return nil, fmt.Errorf("failed to encode invoice provider request: %w", err)
		}
	}

	for attempt := 0; attempt < 2; attempt++ {
		var body io.Reader
		if requestBody != nil {
			body = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
		if err != nil {
			return nil, errors.New("failed to create invoice provider request")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", "application/json")
		if requestBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		response, err := c.httpClient.Do(req)
		if err != nil {
			return nil, c.transportError(err, token)
		}
		if response.StatusCode == http.StatusUnauthorized {
			c.invalidateAccessToken(token)
			if attempt == 0 {
				_ = response.Body.Close()
				token, err = c.accessTokenForRequest(ctx)
				if err != nil {
					return nil, err
				}
				continue
			}
		}
		defer response.Body.Close()
		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			return nil, c.responseError(response, token)
		}

		responseBytes, err := readInvoiceProviderJSONBody(response.Body)
		if err != nil {
			return nil, err
		}
		var envelope invoiceProviderEnvelope[T]
		if err := common.Unmarshal(responseBytes, &envelope); err != nil {
			return nil, fmt.Errorf("invoice provider returned malformed JSON: %w", err)
		}
		code, present := invoiceProviderResponseCode(envelope.Code)
		if !present {
			return nil, errors.New("invoice provider response is missing a business code")
		}
		if code != "0" {
			return nil, c.newProviderError(response.StatusCode, code, envelope.Message, envelope.RequestID, token)
		}
		return &envelope.Data, nil
	}
	return nil, errors.New("invoice provider request failed")
}

func (c *InvoiceProviderClient) accessTokenForRequest(ctx context.Context) (string, error) {
	if c == nil || c.httpClient == nil || c.now == nil {
		return "", errors.New("invoice provider client is not initialized")
	}
	if c.clientID == "" || c.clientSecret == "" {
		return "", errors.New("invoice provider credentials are not configured")
	}

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()

	now := c.now()
	if c.accessToken != "" && now.Add(invoiceProviderTokenRefreshWindow).Before(c.tokenExpiresAt) {
		return c.accessToken, nil
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", c.clientID)
	form.Set("client_secret", c.clientSecret)
	form.Set("scope", invoiceProviderScope)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", errors.New("failed to create invoice provider token request")
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	response, err := c.httpClient.Do(req)
	if err != nil {
		return "", c.transportError(err, "")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", c.responseError(response, "")
	}

	bodyBytes, err := readInvoiceProviderJSONBody(response.Body)
	if err != nil {
		return "", err
	}
	var tokenResponse invoiceProviderTokenResponse
	if err := common.Unmarshal(bodyBytes, &tokenResponse); err != nil {
		return "", fmt.Errorf("invoice provider returned malformed token JSON: %w", err)
	}
	if code, present := invoiceProviderResponseCode(tokenResponse.Code); present && code != "0" {
		return "", c.newProviderError(response.StatusCode, code, tokenResponse.Message, tokenResponse.RequestID, "")
	}

	tokenFields := tokenResponse.invoiceProviderTokenFields
	if strings.TrimSpace(tokenFields.AccessToken) == "" {
		tokenFields = tokenResponse.Data
	}
	token := strings.TrimSpace(tokenFields.AccessToken)
	if token == "" {
		return "", errors.New("invoice provider token response is missing access_token")
	}

	ttl := invoiceProviderTokenTTL(tokenFields.ExpiresIn)
	c.accessToken = token
	c.tokenExpiresAt = now.Add(ttl)
	return token, nil
}

func (c *InvoiceProviderClient) invalidateAccessToken(token string) {
	if c == nil || token == "" {
		return
	}
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.accessToken == token {
		c.accessToken = ""
		c.tokenExpiresAt = time.Time{}
	}
}

func (c *InvoiceProviderClient) responseError(response *http.Response, token string) error {
	bodyBytes, readErr := readInvoiceProviderJSONBody(response.Body)
	if readErr != nil {
		return c.newProviderError(response.StatusCode, "", readErr.Error(), "", token)
	}

	var envelope invoiceProviderErrorEnvelope
	if err := common.Unmarshal(bodyBytes, &envelope); err != nil {
		return c.newProviderError(response.StatusCode, "", http.StatusText(response.StatusCode), "", token)
	}
	code, _ := invoiceProviderResponseCode(envelope.Code)
	if code == "" {
		code = strings.TrimSpace(envelope.OAuthError)
	}
	message := envelope.Message
	if strings.TrimSpace(message) == "" {
		message = envelope.ErrorDescription
	}
	return c.newProviderError(response.StatusCode, code, message, envelope.RequestID, token)
}

func (c *InvoiceProviderClient) newProviderError(statusCode int, code string, message string, requestID string, token string) error {
	return &InvoiceProviderError{
		StatusCode: statusCode,
		Code:       sanitizeInvoiceProviderText(code, c.clientID, c.clientSecret, token),
		Message:    sanitizeInvoiceProviderText(message, c.clientID, c.clientSecret, token),
		RequestID:  sanitizeInvoiceProviderText(requestID, c.clientID, c.clientSecret, token),
	}
}

func (c *InvoiceProviderClient) transportError(err error, token string) error {
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return errors.New("invoice provider transport failed: " + sanitizeInvoiceProviderText(err.Error(), c.clientID, c.clientSecret, token))
}

func readInvoiceProviderJSONBody(reader io.Reader) ([]byte, error) {
	limited := io.LimitReader(reader, invoiceProviderMaxJSONResponseBytes+1)
	bodyBytes, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read invoice provider response: %w", err)
	}
	if int64(len(bodyBytes)) > invoiceProviderMaxJSONResponseBytes {
		return nil, errors.New("invoice provider JSON response is too large")
	}
	return bodyBytes, nil
}

func invoiceProviderResponseCode(value any) (string, bool) {
	switch code := value.(type) {
	case nil:
		return "", false
	case string:
		return strings.TrimSpace(code), true
	case float64:
		return strconv.FormatFloat(code, 'f', -1, 64), true
	case float32:
		return strconv.FormatFloat(float64(code), 'f', -1, 32), true
	case int:
		return strconv.Itoa(code), true
	case int64:
		return strconv.FormatInt(code, 10), true
	case uint64:
		return strconv.FormatUint(code, 10), true
	default:
		return "", false
	}
}

func invoiceProviderTokenTTL(value any) time.Duration {
	var seconds int64
	switch expiresIn := value.(type) {
	case float64:
		seconds = int64(expiresIn)
	case float32:
		seconds = int64(expiresIn)
	case int:
		seconds = int64(expiresIn)
	case int64:
		seconds = expiresIn
	case string:
		seconds, _ = strconv.ParseInt(strings.TrimSpace(expiresIn), 10, 64)
	}
	if seconds <= 0 || seconds > int64((365*24*time.Hour)/time.Second) {
		return invoiceProviderDefaultTokenTTL
	}
	return time.Duration(seconds) * time.Second
}

func validateInvoiceProviderOrderNos(orderNos []string) error {
	if len(orderNos) == 0 || len(orderNos) > invoiceProviderMaxOrderCount {
		return fmt.Errorf("invoice provider requires between 1 and %d order numbers", invoiceProviderMaxOrderCount)
	}
	seen := make(map[string]struct{}, len(orderNos))
	for _, orderNo := range orderNos {
		orderNo = strings.TrimSpace(orderNo)
		if orderNo == "" || len(orderNo) > invoiceProviderMaxOrderNoBytes {
			return errors.New("invoice provider order number is invalid")
		}
		if _, ok := seen[orderNo]; ok {
			return errors.New("invoice provider order numbers must be unique")
		}
		seen[orderNo] = struct{}{}
	}
	return nil
}

func sanitizeInvoiceProviderText(value string, credentials ...string) string {
	value = strings.TrimSpace(value)
	// Replace longer credentials first. Otherwise a client ID that is a prefix
	// of the secret could leave the remainder of the secret in an error string.
	credentials = append([]string(nil), credentials...)
	sort.SliceStable(credentials, func(i, j int) bool {
		return len(credentials[i]) > len(credentials[j])
	})
	for _, credential := range credentials {
		if credential == "" {
			continue
		}
		value = strings.ReplaceAll(value, credential, "[REDACTED]")
		encoded := url.QueryEscape(credential)
		if encoded != credential {
			value = strings.ReplaceAll(value, encoded, "[REDACTED]")
		}
	}
	if len(value) > invoiceProviderMaxErrorTextBytes {
		value = value[:invoiceProviderMaxErrorTextBytes] + "..."
	}
	return value
}
