// Code generated from the Futrou OpenAPI spec. DO NOT EDIT.
// Generated at: 2026-08-30T17:14:09Z
// Source: https://api.futrou.com/v2/openapi.json

package api

import (
	"strings"
	"time"
)

// APIError represents a structured error response from the Futrou API.
type APIError struct {
	Message   string       `json:"message"`
	RequestId string       `json:"requestId"`
	ClientIp  string       `json:"clientIp"`
	Errors    []FieldError `json:"errors"`
}

func (e *APIError) Error() string {
	if len(e.Errors) == 0 {
		return e.Message
	}
	details := make([]string, len(e.Errors))
	for i, fe := range e.Errors {
		details[i] = fe.Message
	}
	return e.Message + ": " + strings.Join(details, "; ")
}

// FieldError is a single validation error within an APIError.
type FieldError struct {
	Message string `json:"message"`
	Code    string `json:"code"`
	Field   string `json:"field"`
}

// LoginResponse is returned from POST /v2/auth/login.
type LoginResponse struct {
	ApiToken ApiToken `json:"apiToken"`
	User     User     `json:"user"`
}

// Activity Activity (Model)
type Activity struct {
	CreatedAt            time.Time              `json:"createdAt,omitempty"`
	ExpiresAt            time.Time              `json:"expiresAt,omitempty"`
	Id                   string                 `json:"id,omitempty"`
	NewValues            map[string]interface{} `json:"newValues,omitempty"`
	OldValues            map[string]interface{} `json:"oldValues,omitempty"`
	ProjectId            string                 `json:"projectId,omitempty"`
	ProjectIdLabel       string                 `json:"projectIdLabel,omitempty"`
	RegionId             string                 `json:"regionId,omitempty"`
	RegionIdLabel        string                 `json:"regionIdLabel,omitempty"`
	ServerId             string                 `json:"serverId,omitempty"`
	ServerIdLabel        string                 `json:"serverIdLabel,omitempty"`
	ServerletId          string                 `json:"serverletId,omitempty"`
	ServerletIdLabel     string                 `json:"serverletIdLabel,omitempty"`
	StorageId            string                 `json:"storageId,omitempty"`
	StorageIdLabel       string                 `json:"storageIdLabel,omitempty"`
	Type                 *ActivityType          `json:"type,omitempty"`
	UpdatedAt            time.Time              `json:"updatedAt,omitempty"`
	User                 *User                  `json:"user,omitempty"`
	UserId               string                 `json:"userId,omitempty"`
	UserIdLabel          string                 `json:"userIdLabel,omitempty"`
	Workspace            *Workspace             `json:"workspace,omitempty"`
	WorkspaceId          string                 `json:"workspaceId,omitempty"`
	WorkspaceIdLabel     string                 `json:"workspaceIdLabel,omitempty"`
	WorkspaceUser        *WorkspaceUser         `json:"workspaceUser,omitempty"`
	WorkspaceUserId      string                 `json:"workspaceUserId,omitempty"`
	WorkspaceUserIdLabel string                 `json:"workspaceUserIdLabel,omitempty"`
}

// ActivityType Activity Type enumeration
type ActivityType string

const (
	ActivityTypeCreated              ActivityType = "created"
	ActivityTypeUpdated              ActivityType = "updated"
	ActivityTypeDeleted              ActivityType = "deleted"
	ActivityTypeUserCreated          ActivityType = "user_created"
	ActivityTypeUserUpdated          ActivityType = "user_updated"
	ActivityTypeUserDeleted          ActivityType = "user_deleted"
	ActivityTypeUserPasswordReset    ActivityType = "user_password_reset"
	ActivityTypeUserPasswordUpdated  ActivityType = "user_password_updated"
	ActivityTypeUserEmailUpdated     ActivityType = "user_email_updated"
	ActivityTypeUserEmailConfirmed   ActivityType = "user_email_confirmed"
	ActivityTypeUserLoginFailed      ActivityType = "user_login_failed"
	ActivityTypeUserLoginSuccess     ActivityType = "user_login_success"
	ActivityTypeUserSsoAdded         ActivityType = "user_sso_added"
	ActivityTypeWorkspaceCreated     ActivityType = "workspace_created"
	ActivityTypeWorkspaceUpdated     ActivityType = "workspace_updated"
	ActivityTypeWorkspaceDeleted     ActivityType = "workspace_deleted"
	ActivityTypeWorkspaceUserCreated ActivityType = "workspace_user_created"
	ActivityTypeWorkspaceUserUpdated ActivityType = "workspace_user_updated"
	ActivityTypeWorkspaceUserDeleted ActivityType = "workspace_user_deleted"
	ActivityTypeOrderCreated         ActivityType = "order_created"
	ActivityTypeOrderCanceled        ActivityType = "order_canceled"
	ActivityTypeVoucherCreated       ActivityType = "voucher_created"
	ActivityTypeVoucherDeleted       ActivityType = "voucher_deleted"
	ActivityTypeVoucherUsed          ActivityType = "voucher_used"
	ActivityTypeDnsCreated           ActivityType = "dns_created"
	ActivityTypeDnsUpdated           ActivityType = "dns_updated"
	ActivityTypeDnsDeleted           ActivityType = "dns_deleted"
	ActivityTypeDnsRecordCreated     ActivityType = "dns_record_created"
	ActivityTypeDnsRecordUpdated     ActivityType = "dns_record_updated"
	ActivityTypeDnsRecordDeleted     ActivityType = "dns_record_deleted"
	ActivityTypeProjectCreated       ActivityType = "project_created"
	ActivityTypeProjectUpdated       ActivityType = "project_updated"
	ActivityTypeProjectDeleted       ActivityType = "project_deleted"
	ActivityTypeProjectTransferred   ActivityType = "project_transferred"
	ActivityTypeProxyCreated         ActivityType = "proxy_created"
	ActivityTypeProxyUpdated         ActivityType = "proxy_updated"
	ActivityTypeProxyDeleted         ActivityType = "proxy_deleted"
	ActivityTypeRegionCreated        ActivityType = "region_created"
	ActivityTypeRegionUpdated        ActivityType = "region_updated"
	ActivityTypeRegionDeleted        ActivityType = "region_deleted"
	ActivityTypeServerCreated        ActivityType = "server_created"
	ActivityTypeServerUpdated        ActivityType = "server_updated"
	ActivityTypeServerDeleted        ActivityType = "server_deleted"
	ActivityTypeServerJoined         ActivityType = "server_joined"
	ActivityTypeServerJoinTokenReset ActivityType = "server_join_token_reset"
	ActivityTypeServerLeft           ActivityType = "server_left"
	ActivityTypeStorageCreated       ActivityType = "storage_created"
	ActivityTypeStorageUpdated       ActivityType = "storage_updated"
	ActivityTypeStorageDeleted       ActivityType = "storage_deleted"
	ActivityTypeServerletCreated     ActivityType = "serverlet_created"
	ActivityTypeServerletUpdated     ActivityType = "serverlet_updated"
	ActivityTypeServerletDeleted     ActivityType = "serverlet_deleted"
	ActivityTypeApiTokenCreated      ActivityType = "api_token_created"
	ActivityTypeApiTokenUpdated      ActivityType = "api_token_updated"
	ActivityTypeApiTokenDeleted      ActivityType = "api_token_deleted"
	ActivityTypeAppCreated           ActivityType = "app_created"
	ActivityTypeAppUpdated           ActivityType = "app_updated"
	ActivityTypeAppDeleted           ActivityType = "app_deleted"
	ActivityTypeVariableCreated      ActivityType = "variable_created"
	ActivityTypeVariableUpdated      ActivityType = "variable_updated"
	ActivityTypeVariableDeleted      ActivityType = "variable_deleted"
	ActivityTypeCertCreated          ActivityType = "cert_created"
	ActivityTypeCertDeleted          ActivityType = "cert_deleted"
	ActivityTypeCertRenewed          ActivityType = "cert_renewed"
	ActivityTypeMonitorCreated       ActivityType = "monitor_created"
	ActivityTypeMonitorUpdated       ActivityType = "monitor_updated"
	ActivityTypeMonitorDeleted       ActivityType = "monitor_deleted"
)

// Any Any JSON object not defined as schema
type Any struct {
}

// ApiToken ApiToken (Model)
type ApiToken struct {
	App           *App           `json:"app,omitempty"`
	AppId         string         `json:"appId,omitempty"`
	CreatedAt     time.Time      `json:"createdAt,omitempty"`
	ExpiresAt     time.Time      `json:"expiresAt,omitempty"`
	Id            string         `json:"id,omitempty"`
	IsTrusted     bool           `json:"isTrusted,omitempty"`
	Name          string         `json:"name,omitempty"`
	Permissions   []string       `json:"permissions,omitempty"`
	Server        *Server        `json:"server,omitempty"`
	ServerId      string         `json:"serverId,omitempty"`
	SsoId         string         `json:"ssoId,omitempty"`
	SsoProvider   string         `json:"ssoProvider,omitempty"`
	Token         string         `json:"token,omitempty"`
	Type          *ApiTokenType  `json:"type,omitempty"`
	UpdatedAt     time.Time      `json:"updatedAt,omitempty"`
	UsedAt        time.Time      `json:"usedAt,omitempty"`
	UsedIp        string         `json:"usedIp,omitempty"`
	UsedUserAgent string         `json:"usedUserAgent,omitempty"`
	User          *User          `json:"user,omitempty"`
	UserId        string         `json:"userId,omitempty"`
	Workspace     *Workspace     `json:"workspace,omitempty"`
	WorkspaceId   string         `json:"workspaceId,omitempty"`
	WorkspaceRole *WorkspaceRole `json:"workspaceRole,omitempty"`
}

// ApiTokenType Api Token Type enumeration
type ApiTokenType string

const (
	ApiTokenTypeUser      ApiTokenType = "user"
	ApiTokenTypeWorkspace ApiTokenType = "workspace"
	ApiTokenTypeServer    ApiTokenType = "server"
)

// App App (Model)
type App struct {
	AllowedPermissions []map[string]interface{} `json:"allowedPermissions,omitempty"`
	ApiTokens          []ApiToken               `json:"apiTokens,omitempty"`
	AuthorizationCodes []OauthAuthorizationCode `json:"authorizationCodes,omitempty"`
	CreatedAt          time.Time                `json:"createdAt,omitempty"`
	DisplayName        string                   `json:"displayName,omitempty"`
	Id                 string                   `json:"id,omitempty"`
	LogoUri            string                   `json:"logoUri,omitempty"`
	Name               string                   `json:"name,omitempty"`
	RedirectUris       []string                 `json:"redirectUris,omitempty"`
	Type               *OAuth2Type              `json:"type,omitempty"`
	UpdatedAt          time.Time                `json:"updatedAt,omitempty"`
}

// Arch Arch enumeration
type Arch string

const (
	ArchAmd64 Arch = "amd64"
	ArchArm64 Arch = "arm64"
)

// Contact Contact (Model)
type Contact struct {
	City           string                 `json:"city,omitempty"`
	Company        string                 `json:"company,omitempty"`
	CompanyNumber  string                 `json:"companyNumber,omitempty"`
	Country        map[string]interface{} `json:"country,omitempty"`
	CreatedAt      time.Time              `json:"createdAt,omitempty"`
	Email          string                 `json:"email,omitempty"`
	Firstname      string                 `json:"firstname,omitempty"`
	Id             string                 `json:"id,omitempty"`
	IsPersonal     bool                   `json:"isPersonal,omitempty"`
	Lastname       string                 `json:"lastname,omitempty"`
	Phone          string                 `json:"phone,omitempty"`
	PostalCode     string                 `json:"postalCode,omitempty"`
	StreetAddress  string                 `json:"streetAddress,omitempty"`
	StreetAddress2 string                 `json:"streetAddress2,omitempty"`
	UpdatedAt      time.Time              `json:"updatedAt,omitempty"`
	VatNumber      string                 `json:"vatNumber,omitempty"`
	Workspace      *Workspace             `json:"workspace,omitempty"`
}

// Cron Cron (Model)
type Cron struct {
	Body        string            `json:"body,omitempty"`
	Code        string            `json:"code,omitempty"`
	CreatedAt   time.Time         `json:"createdAt,omitempty"`
	CronPlan    *CronPlan         `json:"cronPlan,omitempty"`
	CronPlanId  string            `json:"cronPlanId,omitempty"`
	Enabled     bool              `json:"enabled,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Id          string            `json:"id,omitempty"`
	Method      string            `json:"method,omitempty"`
	Name        string            `json:"name,omitempty"`
	Project     *Project          `json:"project,omitempty"`
	ProjectId   string            `json:"projectId,omitempty"`
	Region      *Region           `json:"region,omitempty"`
	RegionId    string            `json:"regionId,omitempty"`
	Schedule    string            `json:"schedule,omitempty"`
	StartedAt   time.Time         `json:"startedAt,omitempty"`
	Type        *CronType         `json:"type,omitempty"`
	UpdatedAt   time.Time         `json:"updatedAt,omitempty"`
	Url         string            `json:"url,omitempty"`
	Workspace   *Workspace        `json:"workspace,omitempty"`
	WorkspaceId string            `json:"workspaceId,omitempty"`
}

// CronPlan CronPlan (Model)
type CronPlan struct {
	CreatedAt    time.Time          `json:"createdAt,omitempty"`
	DisplayName  string             `json:"displayName,omitempty"`
	Id           string             `json:"id,omitempty"`
	IsPublic     bool               `json:"isPublic,omitempty"`
	MinutePrice  map[string]float64 `json:"minutePrice,omitempty"`
	MonthlyPrice map[string]float64 `json:"monthlyPrice,omitempty"`
	Name         string             `json:"name,omitempty"`
	UpdatedAt    time.Time          `json:"updatedAt,omitempty"`
}

// CronType Cron Type enumeration
type CronType string

const (
	CronTypeSimple CronType = "simple"
	CronTypeCode   CronType = "code"
)

// Currency Currency enumeration
type Currency string

const (
	CurrencyEur Currency = "eur"
	CurrencyCzk Currency = "czk"
)

// Invoice Invoice (Model)
type Invoice struct {
	CanceledAt           time.Time      `json:"canceledAt,omitempty"`
	CreatedAt            time.Time      `json:"createdAt,omitempty"`
	Currency             *Currency      `json:"currency,omitempty"`
	CustomerContact      *Contact       `json:"customerContact,omitempty"`
	CustomerContactId    string         `json:"customerContactId,omitempty"`
	DueAt                time.Time      `json:"dueAt,omitempty"`
	ExchangeRate         float64        `json:"exchangeRate,omitempty"`
	ExchangeRateCurrency *Currency      `json:"exchangeRateCurrency,omitempty"`
	Id                   string         `json:"id,omitempty"`
	IssuedAt             time.Time      `json:"issuedAt,omitempty"`
	Items                []InvoiceItem  `json:"items,omitempty"`
	Language             *Language      `json:"language,omitempty"`
	Number               string         `json:"number,omitempty"`
	Order                *Order         `json:"order,omitempty"`
	OrderId              string         `json:"orderId,omitempty"`
	PaidAt               time.Time      `json:"paidAt,omitempty"`
	PdfKey               string         `json:"pdfKey,omitempty"`
	Price                float64        `json:"price,omitempty"`
	ReferenceNumber      string         `json:"referenceNumber,omitempty"`
	RefundedAt           time.Time      `json:"refundedAt,omitempty"`
	Status               *InvoiceStatus `json:"status,omitempty"`
	SupplierContact      *Contact       `json:"supplierContact,omitempty"`
	SupplierContactId    string         `json:"supplierContactId,omitempty"`
	UpdatedAt            time.Time      `json:"updatedAt,omitempty"`
	Workspace            *Workspace     `json:"workspace,omitempty"`
	WorkspaceId          string         `json:"workspaceId,omitempty"`
}

// InvoiceItem InvoiceItem (Interface)
type InvoiceItem struct {
	Description string  `json:"description,omitempty"`
	Quantity    float64 `json:"quantity,omitempty"`
	Unit        string  `json:"unit,omitempty"`
	UnitPrice   float64 `json:"unitPrice,omitempty"`
}

// InvoiceStatus Invoice Status enumeration
type InvoiceStatus string

const (
	InvoiceStatusPending  InvoiceStatus = "pending"
	InvoiceStatusPaid     InvoiceStatus = "paid"
	InvoiceStatusCanceled InvoiceStatus = "canceled"
	InvoiceStatusRefunded InvoiceStatus = "refunded"
)

// Language Language enumeration
type Language string

const (
	LanguageEn Language = "en"
	LanguageCs Language = "cs"
)

// OAuth2Type Oauth2type enumeration
type OAuth2Type string

const (
	OAuth2TypePublic       OAuth2Type = "public"
	OAuth2TypeConfidential OAuth2Type = "confidential"
)

// OauthAuthorizationCode OauthAuthorizationCode (Model)
type OauthAuthorizationCode struct {
	App                 *App                     `json:"app,omitempty"`
	AppId               string                   `json:"appId,omitempty"`
	Code                string                   `json:"code,omitempty"`
	CodeChallenge       string                   `json:"codeChallenge,omitempty"`
	CodeChallengeMethod map[string]interface{}   `json:"codeChallengeMethod,omitempty"`
	CreatedAt           time.Time                `json:"createdAt,omitempty"`
	DeviceName          string                   `json:"deviceName,omitempty"`
	ExpiresAt           time.Time                `json:"expiresAt,omitempty"`
	Id                  string                   `json:"id,omitempty"`
	Permissions         []map[string]interface{} `json:"permissions,omitempty"`
	RedirectUri         string                   `json:"redirectUri,omitempty"`
	UsedAt              time.Time                `json:"usedAt,omitempty"`
	User                *User                    `json:"user,omitempty"`
	UserId              string                   `json:"userId,omitempty"`
	WorkspaceId         string                   `json:"workspaceId,omitempty"`
}

// Order Order (Model)
type Order struct {
	CanceledAt         time.Time         `json:"canceledAt,omitempty"`
	CreatedAt          time.Time         `json:"createdAt,omitempty"`
	CreemCheckoutId    string            `json:"creemCheckoutId,omitempty"`
	CreemProductId     string            `json:"creemProductId,omitempty"`
	CreemTransactionId string            `json:"creemTransactionId,omitempty"`
	Currency           *Currency         `json:"currency,omitempty"`
	Id                 string            `json:"id,omitempty"`
	Invoices           []Invoice         `json:"invoices,omitempty"`
	Number             string            `json:"number,omitempty"`
	PaidAt             time.Time         `json:"paidAt,omitempty"`
	PaymentMethod      *PaymentMethod    `json:"paymentMethod,omitempty"`
	Price              float64           `json:"price,omitempty"`
	ProformaInvoices   []ProformaInvoice `json:"proformaInvoices,omitempty"`
	RefundedAt         time.Time         `json:"refundedAt,omitempty"`
	Status             *OrderStatus      `json:"status,omitempty"`
	UpdatedAt          time.Time         `json:"updatedAt,omitempty"`
	Workspace          *Workspace        `json:"workspace,omitempty"`
	WorkspaceId        string            `json:"workspaceId,omitempty"`
}

// OrderStatus Order Status enumeration
type OrderStatus string

const (
	OrderStatusPending  OrderStatus = "pending"
	OrderStatusPaid     OrderStatus = "paid"
	OrderStatusCanceled OrderStatus = "canceled"
	OrderStatusRefunded OrderStatus = "refunded"
)

// PaymentMethod Payment Method enumeration
type PaymentMethod string

const (
	PaymentMethodBankTransfer PaymentMethod = "bank_transfer"
	PaymentMethodCreem        PaymentMethod = "creem"
	PaymentMethodBarion       PaymentMethod = "barion"
)

// ProformaInvoice ProformaInvoice (Model)
type ProformaInvoice struct {
	CreatedAt            time.Time             `json:"createdAt,omitempty"`
	Currency             *Currency             `json:"currency,omitempty"`
	CustomerContact      *Contact              `json:"customerContact,omitempty"`
	CustomerContactId    string                `json:"customerContactId,omitempty"`
	DueAt                time.Time             `json:"dueAt,omitempty"`
	ExchangeRate         float64               `json:"exchangeRate,omitempty"`
	ExchangeRateCurrency *Currency             `json:"exchangeRateCurrency,omitempty"`
	Id                   string                `json:"id,omitempty"`
	IssuedAt             time.Time             `json:"issuedAt,omitempty"`
	Items                []ProformaInvoiceItem `json:"items,omitempty"`
	Language             *Language             `json:"language,omitempty"`
	Number               string                `json:"number,omitempty"`
	Order                *Order                `json:"order,omitempty"`
	OrderId              string                `json:"orderId,omitempty"`
	PdfKey               string                `json:"pdfKey,omitempty"`
	Price                float64               `json:"price,omitempty"`
	ReferenceNumber      string                `json:"referenceNumber,omitempty"`
	SupplierContact      *Contact              `json:"supplierContact,omitempty"`
	SupplierContactId    string                `json:"supplierContactId,omitempty"`
	UpdatedAt            time.Time             `json:"updatedAt,omitempty"`
	Workspace            *Workspace            `json:"workspace,omitempty"`
	WorkspaceId          string                `json:"workspaceId,omitempty"`
}

// ProformaInvoiceItem ProformaInvoiceItem (Interface)
type ProformaInvoiceItem struct {
	Description string  `json:"description,omitempty"`
	Quantity    float64 `json:"quantity,omitempty"`
	Unit        string  `json:"unit,omitempty"`
	UnitPrice   float64 `json:"unitPrice,omitempty"`
}

// Project Project (Model)
type Project struct {
	CreatedAt         time.Time  `json:"createdAt,omitempty"`
	Crons             []Cron     `json:"crons,omitempty"`
	DisplayName       string     `json:"displayName,omitempty"`
	GitRepositoryName string     `json:"gitRepositoryName,omitempty"`
	GitRepositoryUrl  string     `json:"gitRepositoryUrl,omitempty"`
	Id                string     `json:"id,omitempty"`
	Name              string     `json:"name,omitempty"`
	UpdatedAt         time.Time  `json:"updatedAt,omitempty"`
	Workspace         *Workspace `json:"workspace,omitempty"`
	WorkspaceId       string     `json:"workspaceId,omitempty"`
}

// Proxy Proxy (Model)
type Proxy struct {
	CreatedAt       time.Time      `json:"createdAt,omitempty"`
	Domain          string         `json:"domain,omitempty"`
	EnforceHttps    bool           `json:"enforceHttps,omitempty"`
	FollowRedirects bool           `json:"followRedirects,omitempty"`
	Id              string         `json:"id,omitempty"`
	IsVerified      bool           `json:"isVerified,omitempty"`
	Port            float64        `json:"port,omitempty"`
	PreserveHeaders bool           `json:"preserveHeaders,omitempty"`
	PreserveHost    bool           `json:"preserveHost,omitempty"`
	PreservePath    bool           `json:"preservePath,omitempty"`
	PreserveQuery   bool           `json:"preserveQuery,omitempty"`
	Project         *Project       `json:"project,omitempty"`
	ProjectId       string         `json:"projectId,omitempty"`
	ProxyPlan       *ProxyPlan     `json:"proxyPlan,omitempty"`
	ProxyPlanId     string         `json:"proxyPlanId,omitempty"`
	Region          *Region        `json:"region,omitempty"`
	RegionId        string         `json:"regionId,omitempty"`
	Rules           []Rule         `json:"rules,omitempty"`
	Strategy        *ProxyStrategy `json:"strategy,omitempty"`
	Target          string         `json:"target,omitempty"`
	Type            *ProxyType     `json:"type,omitempty"`
	UpdatedAt       time.Time      `json:"updatedAt,omitempty"`
	VerifyTls       bool           `json:"verifyTls,omitempty"`
	Workspace       *Workspace     `json:"workspace,omitempty"`
	WorkspaceId     string         `json:"workspaceId,omitempty"`
}

// ProxyPlan ProxyPlan (Model)
type ProxyPlan struct {
	CreatedAt    time.Time          `json:"createdAt,omitempty"`
	DisplayName  string             `json:"displayName,omitempty"`
	Id           string             `json:"id,omitempty"`
	IsPublic     bool               `json:"isPublic,omitempty"`
	MinutePrice  map[string]float64 `json:"minutePrice,omitempty"`
	MonthlyPrice map[string]float64 `json:"monthlyPrice,omitempty"`
	Name         string             `json:"name,omitempty"`
	UpdatedAt    time.Time          `json:"updatedAt,omitempty"`
}

// ProxyStrategy Proxy Strategy enumeration
type ProxyStrategy string

const (
	ProxyStrategyRoundRobin      ProxyStrategy = "round-robin"
	ProxyStrategyPrimaryFailover ProxyStrategy = "primary-failover"
)

// ProxyType Proxy Type enumeration
type ProxyType string

const (
	ProxyTypeHttp ProxyType = "http"
	ProxyTypeTcp  ProxyType = "tcp"
	ProxyTypeUdp  ProxyType = "udp"
)

// Region Region (Model)
type Region struct {
	CreatedAt   time.Time  `json:"createdAt,omitempty"`
	DisplayName string     `json:"displayName,omitempty"`
	Domain      string     `json:"domain,omitempty"`
	Id          string     `json:"id,omitempty"`
	IsPublic    bool       `json:"isPublic,omitempty"`
	Name        string     `json:"name,omitempty"`
	Servers     []Server   `json:"servers,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt,omitempty"`
	Workspace   *Workspace `json:"workspace,omitempty"`
	WorkspaceId string     `json:"workspaceId,omitempty"`
}

// Rule Rule (Interface)
type Rule struct {
	If   *RuleCondition           `json:"if,omitempty"`
	Then []map[string]interface{} `json:"then,omitempty"`
}

// RuleCondition RuleCondition (Interface)
type RuleCondition struct {
	Host   string      `json:"host,omitempty"`
	Method *RuleMethod `json:"method,omitempty"`
	Path   string      `json:"path,omitempty"`
	Port   float64     `json:"port,omitempty"`
}

// RuleMethod Rule Method enumeration
type RuleMethod string

const (
	RuleMethodGet     RuleMethod = "get"
	RuleMethodPost    RuleMethod = "post"
	RuleMethodPut     RuleMethod = "put"
	RuleMethodDelete  RuleMethod = "delete"
	RuleMethodPatch   RuleMethod = "patch"
	RuleMethodOptions RuleMethod = "options"
	RuleMethodHead    RuleMethod = "head"
)

// Server Server (Model)
type Server struct {
	ApiCertHash string     `json:"apiCertHash,omitempty"`
	ApiHost     string     `json:"apiHost,omitempty"`
	ApiPort     string     `json:"apiPort,omitempty"`
	ApiTokens   []ApiToken `json:"apiTokens,omitempty"`
	Arch        *Arch      `json:"arch,omitempty"`
	CreatedAt   time.Time  `json:"createdAt,omitempty"`
	DisplayName string     `json:"displayName,omitempty"`
	Domain      string     `json:"domain,omitempty"`
	Id          string     `json:"id,omitempty"`
	IsPublic    bool       `json:"isPublic,omitempty"`
	JoinedAt    time.Time  `json:"joinedAt,omitempty"`
	Name        string     `json:"name,omitempty"`
	Region      *Region    `json:"region,omitempty"`
	RegionId    string     `json:"regionId,omitempty"`
	SeenAt      time.Time  `json:"seenAt,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt,omitempty"`
	WorkspaceId string     `json:"workspaceId,omitempty"`
}

// Serverlet Serverlet (Model)
type Serverlet struct {
	Arch            *Arch              `json:"arch,omitempty"`
	CreatedAt       time.Time          `json:"createdAt,omitempty"`
	Id              string             `json:"id,omitempty"`
	Image           string             `json:"image,omitempty"`
	MaxInstances    float64            `json:"maxInstances,omitempty"`
	MinInstances    float64            `json:"minInstances,omitempty"`
	Mounts          map[string]string  `json:"mounts,omitempty"`
	Name            string             `json:"name,omitempty"`
	Project         *Project           `json:"project,omitempty"`
	ProjectId       string             `json:"projectId,omitempty"`
	Region          *Region            `json:"region,omitempty"`
	RegionId        string             `json:"regionId,omitempty"`
	Scaling         map[string]float64 `json:"scaling,omitempty"`
	ServerletPlan   *ServerletPlan     `json:"serverletPlan,omitempty"`
	ServerletPlanId string             `json:"serverletPlanId,omitempty"`
	Status          *ServerletStatus   `json:"status,omitempty"`
	UpdatedAt       time.Time          `json:"updatedAt,omitempty"`
	Workspace       *Workspace         `json:"workspace,omitempty"`
	WorkspaceId     string             `json:"workspaceId,omitempty"`
}

// ServerletPlan ServerletPlan (Model)
type ServerletPlan struct {
	Capacity     map[string]interface{} `json:"capacity,omitempty"`
	Cpu          float64                `json:"cpu,omitempty"`
	CreatedAt    time.Time              `json:"createdAt,omitempty"`
	DisplayName  string                 `json:"displayName,omitempty"`
	Id           string                 `json:"id,omitempty"`
	IsPublic     bool                   `json:"isPublic,omitempty"`
	MinutePrice  map[string]float64     `json:"minutePrice,omitempty"`
	MonthlyPrice map[string]float64     `json:"monthlyPrice,omitempty"`
	Name         string                 `json:"name,omitempty"`
	Ram          map[string]interface{} `json:"ram,omitempty"`
	UpdatedAt    time.Time              `json:"updatedAt,omitempty"`
}

// ServerletStatus Serverlet Status enumeration
type ServerletStatus string

const (
	ServerletStatusPending  ServerletStatus = "pending"
	ServerletStatusCreating ServerletStatus = "creating"
	ServerletStatusReady    ServerletStatus = "ready"
	ServerletStatusDeleting ServerletStatus = "deleting"
	ServerletStatusDeleted  ServerletStatus = "deleted"
	ServerletStatusFailed   ServerletStatus = "failed"
)

// StaffRole Staff roles, keyed in hierarchy order from lowest to highest privilege.
type StaffRole string

const (
	StaffRoleNone          StaffRole = "none"
	StaffRoleViewer        StaffRole = "viewer"
	StaffRoleDeveloper     StaffRole = "developer"
	StaffRoleAdministrator StaffRole = "administrator"
	StaffRoleOwner         StaffRole = "owner"
)

// Storage Storage (Model)
type Storage struct {
	CreatedAt     time.Time    `json:"createdAt,omitempty"`
	Id            string       `json:"id,omitempty"`
	Name          string       `json:"name,omitempty"`
	Project       *Project     `json:"project,omitempty"`
	ProjectId     string       `json:"projectId,omitempty"`
	Public        bool         `json:"public,omitempty"`
	Region        *Region      `json:"region,omitempty"`
	RegionId      string       `json:"regionId,omitempty"`
	StoragePlan   *StoragePlan `json:"storagePlan,omitempty"`
	StoragePlanId string       `json:"storagePlanId,omitempty"`
	UpdatedAt     time.Time    `json:"updatedAt,omitempty"`
	Workspace     *Workspace   `json:"workspace,omitempty"`
	WorkspaceId   string       `json:"workspaceId,omitempty"`
}

// StoragePlan StoragePlan (Model)
type StoragePlan struct {
	CreatedAt    time.Time              `json:"createdAt,omitempty"`
	DisplayName  string                 `json:"displayName,omitempty"`
	Id           string                 `json:"id,omitempty"`
	IsPublic     bool                   `json:"isPublic,omitempty"`
	MinutePrice  map[string]float64     `json:"minutePrice,omitempty"`
	MonthlyPrice map[string]float64     `json:"monthlyPrice,omitempty"`
	Name         string                 `json:"name,omitempty"`
	Size         map[string]interface{} `json:"size,omitempty"`
	UpdatedAt    time.Time              `json:"updatedAt,omitempty"`
}

// Transaction Transaction (Model)
type Transaction struct {
	CreatedAt   time.Time  `json:"createdAt,omitempty"`
	Currency    *Currency  `json:"currency,omitempty"`
	Description string     `json:"description,omitempty"`
	Id          string     `json:"id,omitempty"`
	Price       float64    `json:"price,omitempty"`
	Project     *Project   `json:"project,omitempty"`
	ProjectId   string     `json:"projectId,omitempty"`
	Type        string     `json:"type,omitempty"`
	Unit        string     `json:"unit,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt,omitempty"`
	Value       float64    `json:"value,omitempty"`
	Workspace   *Workspace `json:"workspace,omitempty"`
	WorkspaceId string     `json:"workspaceId,omitempty"`
}

// TwoFaType Two Fa Type enumeration
type TwoFaType string

const (
	TwoFaTypeOtpApp   TwoFaType = "otp_app"
	TwoFaTypeOtpEmail TwoFaType = "otp_email"
)

// User User (Model)
type User struct {
	ActiveStaffRole   *StaffRole               `json:"activeStaffRole,omitempty"`
	Activities        []Activity               `json:"activities,omitempty"`
	ApiTokens         []ApiToken               `json:"apiTokens,omitempty"`
	CreatedAt         time.Time                `json:"createdAt,omitempty"`
	Email             string                   `json:"email,omitempty"`
	Fullname          string                   `json:"fullname,omitempty"`
	Id                string                   `json:"id,omitempty"`
	IsEmailConfirmed  bool                     `json:"isEmailConfirmed,omitempty"`
	IsStaff           bool                     `json:"isStaff,omitempty"`
	OauthAccessTokens []OauthAuthorizationCode `json:"oauthAccessTokens,omitempty"`
	RegistrationIp    string                   `json:"registrationIp,omitempty"`
	SsoAvatarUrl      string                   `json:"ssoAvatarUrl,omitempty"`
	SsoGithubId       string                   `json:"ssoGithubId,omitempty"`
	SsoGitlabId       string                   `json:"ssoGitlabId,omitempty"`
	SsoGoogleId       string                   `json:"ssoGoogleId,omitempty"`
	SsoMicrosoftId    string                   `json:"ssoMicrosoftId,omitempty"`
	SsoZohoId         string                   `json:"ssoZohoId,omitempty"`
	StaffRole         *StaffRole               `json:"staffRole,omitempty"`
	TwoFaBackupCodes  []string                 `json:"twoFaBackupCodes,omitempty"`
	TwoFaType         *TwoFaType               `json:"twoFaType,omitempty"`
	UpdatedAt         time.Time                `json:"updatedAt,omitempty"`
	WorkspaceUsers    []WorkspaceUser          `json:"workspaceUsers,omitempty"`
	Workspaces        *Any                     `json:"workspaces,omitempty"`
}

// Variable Variable (Model)
type Variable struct {
	CreatedAt   time.Time  `json:"createdAt,omitempty"`
	Description string     `json:"description,omitempty"`
	Id          string     `json:"id,omitempty"`
	IsSecret    bool       `json:"isSecret,omitempty"`
	Key         string     `json:"key,omitempty"`
	Project     *Project   `json:"project,omitempty"`
	ProjectId   string     `json:"projectId,omitempty"`
	Serverlet   *Serverlet `json:"serverlet,omitempty"`
	ServerletId string     `json:"serverletId,omitempty"`
	UpdatedAt   time.Time  `json:"updatedAt,omitempty"`
	Workspace   *Workspace `json:"workspace,omitempty"`
	WorkspaceId string     `json:"workspaceId,omitempty"`
}

// Voucher Voucher (Model)
type Voucher struct {
	Code              string     `json:"code,omitempty"`
	CreatedAt         time.Time  `json:"createdAt,omitempty"`
	Currency          *Currency  `json:"currency,omitempty"`
	ExpiresAt         time.Time  `json:"expiresAt,omitempty"`
	Id                string     `json:"id,omitempty"`
	IsExpired         bool       `json:"isExpired,omitempty"`
	IsUsed            bool       `json:"isUsed,omitempty"`
	Price             float64    `json:"price,omitempty"`
	UpdatedAt         time.Time  `json:"updatedAt,omitempty"`
	UsedAt            time.Time  `json:"usedAt,omitempty"`
	UsedByWorkspace   *Workspace `json:"usedByWorkspace,omitempty"`
	UsedByWorkspaceId string     `json:"usedByWorkspaceId,omitempty"`
	Workspace         *Workspace `json:"workspace,omitempty"`
	WorkspaceId       string     `json:"workspaceId,omitempty"`
}

// Workspace Workspace (Model)
type Workspace struct {
	Activities          []Activity      `json:"activities,omitempty"`
	ApiTokensLimit      float64         `json:"apiTokensLimit,omitempty"`
	AutoJoinDomain      string          `json:"autoJoinDomain,omitempty"`
	AutoJoinRole        *WorkspaceRole  `json:"autoJoinRole,omitempty"`
	Contact             *Contact        `json:"contact,omitempty"`
	ContactId           string          `json:"contactId,omitempty"`
	CreatedAt           time.Time       `json:"createdAt,omitempty"`
	Credit              float64         `json:"credit,omitempty"`
	Crons               []Cron          `json:"crons,omitempty"`
	CronsLimit          float64         `json:"cronsLimit,omitempty"`
	Currency            *Currency       `json:"currency,omitempty"`
	DisplayName         string          `json:"displayName,omitempty"`
	DnsLimit            float64         `json:"dnsLimit,omitempty"`
	FeatureFlags        []string        `json:"featureFlags,omitempty"`
	Id                  string          `json:"id,omitempty"`
	Language            *Language       `json:"language,omitempty"`
	Name                string          `json:"name,omitempty"`
	Number              float64         `json:"number,omitempty"`
	Orders              []Order         `json:"orders,omitempty"`
	Projects            []Project       `json:"projects,omitempty"`
	ProjectsLimit       float64         `json:"projectsLimit,omitempty"`
	ProxiesLimit        float64         `json:"proxiesLimit,omitempty"`
	ServerletsLimit     float64         `json:"serverletsLimit,omitempty"`
	ServersLimit        float64         `json:"serversLimit,omitempty"`
	StorageLimit        float64         `json:"storageLimit,omitempty"`
	Transactions        []Transaction   `json:"transactions,omitempty"`
	UpdatedAt           time.Time       `json:"updatedAt,omitempty"`
	Users               *Any            `json:"users,omitempty"`
	Vouchers            []Voucher       `json:"vouchers,omitempty"`
	WorkspaceUsers      []WorkspaceUser `json:"workspaceUsers,omitempty"`
	WorkspaceUsersLimit float64         `json:"workspaceUsersLimit,omitempty"`
}

// WorkspaceRole Staff role hierarchy - ordered from lowest to highest privilege Workspace roles, keyed in hierarchy order from lowest to highest privilege.
type WorkspaceRole string

const (
	WorkspaceRoleNone           WorkspaceRole = "none"
	WorkspaceRoleViewer         WorkspaceRole = "viewer"
	WorkspaceRoleDeveloper      WorkspaceRole = "developer"
	WorkspaceRoleBillingManager WorkspaceRole = "billing_manager"
	WorkspaceRoleAdministrator  WorkspaceRole = "administrator"
	WorkspaceRoleOwner          WorkspaceRole = "owner"
)

// WorkspaceUser WorkspaceUser (Model)
type WorkspaceUser struct {
	CreatedAt   time.Time      `json:"createdAt,omitempty"`
	Id          string         `json:"id,omitempty"`
	ProjectIds  []string       `json:"projectIds,omitempty"`
	Role        *WorkspaceRole `json:"role,omitempty"`
	UpdatedAt   time.Time      `json:"updatedAt,omitempty"`
	User        *User          `json:"user,omitempty"`
	UserId      string         `json:"userId,omitempty"`
	Workspace   *Workspace     `json:"workspace,omitempty"`
	WorkspaceId string         `json:"workspaceId,omitempty"`
}
