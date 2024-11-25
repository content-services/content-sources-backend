package config

import "net/url"

// EnvironmentPrefix create the prefix for the candlepin environment for a template
//
// Red Hat content is appended to an environment prefix
//
//	for example "content/dist/rhel9/9.4/x86_64/appstream/os/"
//	would end up prefixed with  /api/pulp/content/$REDHAT_DOMAIN/templates/$TEMPLATE_UUID
func EnvironmentPrefix(rhContentPath, rhDomainName, templateUUID string) (string, error) {
	return url.JoinPath(rhContentPath, rhDomainName, "templates", templateUUID)
}
