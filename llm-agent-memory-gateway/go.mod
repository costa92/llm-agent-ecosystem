module github.com/costa92/llm-agent-memory-gateway

go 1.26.0

require (
	github.com/costa92/llm-agent-memory v0.0.0
	github.com/costa92/llm-agent-memory-postgres v0.0.0
	github.com/costa92/llm-agent-rag v1.0.5
	github.com/jackc/pgx/v5 v5.9.2
)

require (
	github.com/costa92/llm-agent v0.7.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/pgvector/pgvector-go v0.3.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/x448/float16 v0.8.4 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.35.0 // indirect
	modernc.org/libc v1.72.3 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
	modernc.org/sqlite v1.50.1 // indirect
)

replace github.com/costa92/llm-agent-memory => ../llm-agent-memory

replace github.com/costa92/llm-agent-memory-postgres => ../llm-agent-memory-postgres

replace github.com/costa92/llm-agent-rag => ../llm-agent-rag
