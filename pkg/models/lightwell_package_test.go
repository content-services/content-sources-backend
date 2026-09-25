package models

import "testing"

func TestLightwellPackageTableNames(t *testing.T) {
	pkg := LightwellPackage{}
	if pkg.TableName() != "lightwell_packages" {
		t.Fatalf("unexpected table name: %s", pkg.TableName())
	}
	pkgVer := LightwellPackageVersion{}
	if pkgVer.TableName() != "lightwell_package_versions" {
		t.Fatalf("unexpected table name: %s", pkgVer.TableName())
	}
}
