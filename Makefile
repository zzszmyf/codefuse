.PHONY: all build test lint coverage clean install fmt vet check pre-commit fixtures bench-profile

BINARY := codefuse
CMD := ./cmd/codefuse

all: fmt vet test build

build:
	go build -o $(BINARY) $(CMD)

test:
	go test -race ./...

test-coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage:"
	@go tool cover -func=coverage.out | tail -1

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout=5m; \
	else \
		echo "golangci-lint not installed. Run: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

fmt:
	go fmt ./...

vet:
	go vet ./...

# Quick pre-commit check — fast, catches 90% of issues.
check: fmt vet
	go test -short ./...
	go build ./cmd/codefuse

# Pre-commit: full check (slower, for CI).
pre-commit: fmt vet
	go test -race ./...
	go build ./cmd/codefuse

# Regenerate tree-sitter XML test fixtures.
fixtures:
	@mkdir -p internal/parser/testdata
	@echo 'import sql' > /tmp/_cf_py.py
	@echo 'from db.dao import UserDao' >> /tmp/_cf_py.py
	@echo '' >> /tmp/_cf_py.py
	@echo 'class AuthService:' >> /tmp/_cf_py.py
	@echo '    def login(self, token: str):' >> /tmp/_cf_py.py
	@echo '        dao = UserDao()' >> /tmp/_cf_py.py
	@echo '        result = dao.findById(token)' >> /tmp/_cf_py.py
	@echo '        sql.Query("SELECT * FROM users")' >> /tmp/_cf_py.py
	@echo '        return result' >> /tmp/_cf_py.py
	@echo '' >> /tmp/_cf_py.py
	@echo 'def helper(x: int):' >> /tmp/_cf_py.py
	@echo '    return x + 1' >> /tmp/_cf_py.py
	tree-sitter parse --xml /tmp/_cf_py.py > internal/parser/testdata/fixture_py.xml
	@echo 'import com.foo.UserDao;' > /tmp/_cf_java.java
	@echo 'import java.sql.Connection;' >> /tmp/_cf_java.java
	@echo '' >> /tmp/_cf_java.java
	@echo 'class AuthService {' >> /tmp/_cf_java.java
	@echo '    String login(String token) {' >> /tmp/_cf_java.java
	@echo '        UserDao dao = new UserDao();' >> /tmp/_cf_java.java
	@echo '        String result = dao.findById(token);' >> /tmp/_cf_java.java
	@echo '        return result;' >> /tmp/_cf_java.java
	@echo '    }' >> /tmp/_cf_java.java
	@echo '}' >> /tmp/_cf_java.java
	tree-sitter parse --xml /tmp/_cf_java.java > internal/parser/testdata/fixture_java.xml
	@echo "" | tree-sitter parse --xml -- /dev/stdin > internal/parser/testdata/fixture_empty.xml 2>/dev/null || echo '<sources/>' > internal/parser/testdata/fixture_empty.xml
	@echo "Fixtures updated."

# Profile query performance.
bench-profile:
	go build -o /tmp/codefuse ./cmd/codefuse
	@echo "=== Phase breakdown ==="
	@cd /Users/yifanmeng/Project/dubbo 2>/dev/null && for i in 1 2 3; do \
		s=$$(perl -MTime::HiRes=time -e 'printf "%d\n", time*1000'); \
		/tmp/codefuse grep -c RegistryService . > /dev/null 2>&1; \
		e=$$(perl -MTime::HiRes=time -e 'printf "%d\n", time*1000'); \
		echo "  query $$i: $$((e-s))ms"; \
	done || echo "  (no dubbo index — run 'make fixtures' first)"

clean:
	rm -f $(BINARY) coverage.out coverage.html

install:
	go install $(CMD)

bench:
	go test -bench=. -benchmem ./...
