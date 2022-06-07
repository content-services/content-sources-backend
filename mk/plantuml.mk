##
# Rules related with the generation of plantuml diagrams.
#
# NOTE: Keep in mind that they don't need to be added to the
#       repository as it can be seen at the link below:
#       https://blog.anoff.io/2018-07-31-diagrams-with-plantuml/
##

.PHONY: diagrams
plantuml-generate: PLANTUML ?= $(shell command -v plantuml 2>/dev/null)
plantuml-generate: PLANTUML ?= false
plantuml-generate: $(patsubst docs/%.puml,docs/%.svg,$(wildcard docs/*.puml)) ## Generate diagrams

# General rule to generate a diagram in SVG format for 
# each .puml file found at docs/ directory
docs/%.svg: docs/%.puml
	plantuml -tsvg $<
