package validators

import (
	"math"
)

// ConfigValidator validates the top-level declarative Futrou config. It is a
// constructed validator so commands and future config consumers share exactly
// the same schema and fluent validation rules.
var ConfigValidator = initConfigValidator()

func initConfigValidator() *Validator {
	v := NewValidator().Description("Declarative Futrou project configuration.")
	v.Field("$schema").Description("URL of the Futrou JSON Schema used by editors for validation and completion.").Optional().String().URL()
	v.Field("workspace").Description("Workspace name or ID that owns this project.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("project").Description("Project name or ID used as the deployment target.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("serverlets").Description("Serverlets to create or update in this project.").Optional().Array(initServerletValidator())
	v.Field("dns").Description("DNS zones and records managed by this project.").Optional().Array(initDNSValidator())
	v.Field("proxies").Description("HTTP, TCP, or UDP proxies managed by this project.").Optional().Array(initProxyValidator())
	v.Field("storages").Description("Persistent storage volumes available to the project.").Optional().Array(initStorageValidator())
	v.Field("crons").Description("Scheduled HTTP or code jobs for the project.").Optional().Array(initCronValidator())
	v.Field("locks").Description("Internal stable resource identifiers maintained by the CLI.").Optional()
	return v
}

func initResourceValidator() *Validator {
	v := NewValidator().Description("Common Futrou API resource fields.")
	v.Field("id").Description("Futrou resource identifier.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("name").Description("Resource name.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("displayName").Description("Human-readable resource name.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("domain").Description("Domain name assigned to the resource.").Optional().String().MinLength(1).MaxLength(255)
	return v
}

func initServerletValidator() *Validator {
	v := initResourceValidator()
	v.Field("image").Description("Container image to run.").Optional().String().MinLength(1)
	v.Field("serverletPlanId").Description("Plan ID that determines serverlet sizing.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("createdAt").Optional().String()
	v.Field("updatedAt").Optional().String()
	v.Field("status").Optional().String()
	v.Field("workspaceId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("projectId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("regionId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("arch").Optional()
	v.Field("env").Optional()
	v.Field("mounts").Description("Storage mounts, keyed by mount path to storage ID.").Optional()
	v.Field("ports").Optional()
	v.Field("scaling").Optional()
	return v
}

func initProxyValidator() *Validator {
	v := initResourceValidator()
	v.Field("type").Optional().String().Enum("http", "tcp", "udp")
	v.Field("target").Optional().String().MinLength(1)
	v.Field("port").Description("Public proxy port (1–65535).").Optional().Integer().Range(1, 65535)
	v.Field("proxyPlanId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("regionId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("enforceHttps").Optional().Bool()
	v.Field("followRedirects").Optional().Bool()
	v.Field("preserveHeaders").Optional().Bool()
	v.Field("preserveHost").Optional().Bool()
	v.Field("preservePath").Optional().Bool()
	v.Field("preserveQuery").Optional().Bool()
	v.Field("verifyTls").Optional().Bool()
	v.Field("isVerified").Optional().Bool()
	v.Field("status").Optional().String()
	v.Field("strategy").Optional().String().Enum("round-robin", "primary-failover")
	v.Field("rules").Optional()
	v.Field("workspaceId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("projectId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("createdAt").Optional().String()
	v.Field("updatedAt").Optional().String()
	return v
}

func initDNSValidator() *Validator {
	v := initResourceValidator()
	v.Field("ttl").Description("DNS time-to-live in seconds.").Optional().Integer().Range(0, math.MaxInt32)
	v.Field("priority").Description("DNS record priority.").Optional().Integer().Range(0, 65535)
	v.Field("workspaceId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("projectId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("records").Optional().Array(initDNSRecordValidator())
	return v
}

func initDNSRecordValidator() *Validator {
	v := initResourceValidator()
	v.Field("type").Optional().String()
	v.Field("value").Optional().String()
	v.Field("ttl").Optional().Integer().Range(0, math.MaxInt32)
	v.Field("priority").Optional().Integer().Range(0, 65535)
	return v
}

func initStorageValidator() *Validator {
	v := initResourceValidator()
	v.Field("storagePlanId").Description("Plan ID that determines storage sizing.").Optional().String().MinLength(1).MaxLength(255)
	v.Field("workspaceId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("projectId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("regionId").Optional().String().MinLength(1).MaxLength(255)
	v.Field("public").Optional().Bool()
	v.Field("createdAt").Optional().String()
	v.Field("updatedAt").Optional().String()
	return v
}

func initCronValidator() *Validator {
	v := initResourceValidator()
	v.Field("body").Optional().String()
	v.Field("code").Optional().String()
	v.Field("cronPlanId").Optional().String()
	v.Field("enabled").Optional().Bool()
	v.Field("headers").Optional()
	v.Field("method").Optional().String()
	v.Field("projectId").Optional().String()
	v.Field("regionId").Optional().String()
	v.Field("schedule").Optional().String()
	v.Field("url").Optional().String().URL()
	v.Field("workspaceId").Optional().String()
	v.Field("createdAt").Optional().String().DateTime()
	v.Field("startedAt").Optional().String().DateTime()
	v.Field("updatedAt").Optional().String().DateTime()
	v.Field("cronPlan").Optional()
	v.Field("project").Optional()
	v.Field("region").Optional()
	v.Field("type").Optional()
	v.Field("workspace").Optional()
	return v
}
