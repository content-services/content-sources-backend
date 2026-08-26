package parser

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	gitpom "github.com/git-pkgs/pom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type errFetcher struct{}

func (errFetcher) Fetch(_ context.Context, gav gitpom.GAV) (*gitpom.POM, error) {
	return nil, fmt.Errorf("offline test fetcher: %s", gav)
}

func TestMain(m *testing.M) {
	pomFetcher = errFetcher{}
	os.Exit(m.Run())
}

func TestDetectFormat_POM(t *testing.T) {
	cases := []string{"pom.xml", "POM.XML", "path/to/pom.xml", "foo-1.0.pom"}
	for _, name := range cases {
		format, err := detectFormat(name, bufio.NewReader(strings.NewReader("")))
		require.NoError(t, err, name)
		assert.Equal(t, FormatPOM, format, name)
	}
}

func TestParse_POM(t *testing.T) {
	data := `
<project>
  <groupId>com.example</groupId>
  <artifactId>app</artifactId>
  <version>1.0.0</version>
  <dependencies>
    <dependency>
      <groupId>org.springframework</groupId>
      <artifactId>spring-core</artifactId>
      <version>5.3.20</version>
    </dependency>
    <dependency>
      <groupId>junit</groupId>
      <artifactId>junit</artifactId>
      <version>4.13.2</version>
      <scope>test</scope>
    </dependency>
  </dependencies>
</project>`

	result, err := Parse("pom.xml", strings.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, FormatPOM, result.InputFormat)
	assert.Equal(t, []Package{
		{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: "5.3.20"},
		{Ecosystem: EcosystemJava, Namespace: "junit", Name: "junit", Version: "4.13.2"},
	}, result.Packages)
}

func TestParse_POMOnlyProjectDependencies(t *testing.T) {
	data := `
<project>
  <groupId>com.example</groupId>
  <artifactId>app</artifactId>
  <version>1.0.0</version>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>org.apache.commons</groupId>
        <artifactId>commons-lang3</artifactId>
        <version>3.12.0</version>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <dependencies>
    <dependency>
      <groupId>org.slf4j</groupId>
      <artifactId>slf4j-api</artifactId>
      <version>1.7.36</version>
    </dependency>
  </dependencies>
  <build>
    <plugins>
      <plugin>
        <artifactId>maven-compiler-plugin</artifactId>
        <dependencies>
          <dependency>
            <groupId>org.codehaus.plexus</groupId>
            <artifactId>plexus-compiler-javac</artifactId>
            <version>2.13.0</version>
          </dependency>
        </dependencies>
      </plugin>
    </plugins>
  </build>
</project>`

	result, err := Parse("pom.xml", strings.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, []Package{
		{Ecosystem: EcosystemJava, Namespace: "org.slf4j", Name: "slf4j-api", Version: "1.7.36"},
	}, result.Packages)
}

func TestParse_POMUnresolvedVersionsAreEmpty(t *testing.T) {
	data := `
<project>
  <groupId>com.example</groupId>
  <artifactId>app</artifactId>
  <version>1.0.0</version>
  <dependencyManagement>
    <dependencies>
      <dependency>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-dependencies</artifactId>
        <version>2.7.0</version>
        <type>pom</type>
        <scope>import</scope>
      </dependency>
    </dependencies>
  </dependencyManagement>
  <dependencies>
    <dependency>
      <groupId>org.springframework</groupId>
      <artifactId>spring-core</artifactId>
      <version>${spring.version}</version>
    </dependency>
    <dependency>
      <groupId>org.springframework.boot</groupId>
      <artifactId>spring-boot-starter</artifactId>
    </dependency>
  </dependencies>
</project>`

	result, err := Parse("pom.xml", strings.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, []Package{
		{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: ""},
		{Ecosystem: EcosystemJava, Namespace: "org.springframework.boot", Name: "spring-boot-starter", Version: ""},
	}, result.Packages)
}

func TestParse_POMRejectsBadInput(t *testing.T) {
	_, err := Parse("pom.xml", strings.NewReader("<project><dependencies>"))
	assert.Error(t, err)

	_, err = Parse("app.pom", strings.NewReader("<module></module>"))
	assert.Error(t, err)
}

func TestPackagesFromEffective(t *testing.T) {
	pkgs := packagesFromEffective(&gitpom.EffectivePOM{
		Dependencies: []gitpom.ResolvedDep{
			{GroupID: "org.slf4j", ArtifactID: "slf4j-api", Version: "1.7.36", Resolution: gitpom.Resolved},
			{GroupID: "junit", ArtifactID: "junit", Version: "4.13.2", Scope: "test", Resolution: gitpom.Resolved},
			{GroupID: "org.springframework", ArtifactID: "spring-core", Version: "${spring.version}", Resolution: gitpom.UnresolvedProperty, Expression: "${spring.version}"},
			{GroupID: "org.springframework.boot", ArtifactID: "spring-boot-dependencies", Scope: "import", Version: "2.7.0", Resolution: gitpom.Resolved},
			{GroupID: "", ArtifactID: "no-group", Version: "1.0", Resolution: gitpom.Resolved},
			{GroupID: "${project.groupId}", ArtifactID: "lib", Version: "1.0", Resolution: gitpom.UnresolvedProperty, Expression: "${project.groupId}"},
		},
	})
	assert.Equal(t, []Package{
		{Ecosystem: EcosystemJava, Namespace: "org.slf4j", Name: "slf4j-api", Version: "1.7.36"},
		{Ecosystem: EcosystemJava, Namespace: "junit", Name: "junit", Version: "4.13.2"},
		{Ecosystem: EcosystemJava, Namespace: "org.springframework", Name: "spring-core", Version: ""},
	}, pkgs)
}
