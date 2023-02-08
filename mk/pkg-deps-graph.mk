# More information on generating the graph: https://github.com/loov/goda

GRAPH_PATH ?= $(PROJECT_DIR)/docs/pkg-graph.svg

.PHONY: pkg-graph-generate
pkg-graph-generate: ## Generate graph of non-external package dependencies
	mkdir _temp_pkg_graph && cd _temp_pkg_graph; \
    git clone https://github.com/loov/goda && cd goda; \
    go get github.com/content-services/content-sources-backend; \
    sed -i '4i replace github.com/content-services/content-sources-backend => ../../' go.mod; \
    go mod tidy; \
    go get github.com/content-services/content-sources-backend; \
    go run main.go graph -cluster -short github.com/content-services/content-sources-backend/... | dot -Tsvg -o $(GRAPH_PATH); \
    cd ../../ && rm -rf _temp_pkg_graph/
