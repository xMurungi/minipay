DB_URL=postgres://postgres:example@localhost:5432/minipay?sslmode=disable

up:        ; docker compose up -d db
migrate:   ; goose -dir migrations postgres "$(DB_URL)" up
reset:     ; goose -dir migrations postgres "$(DB_URL)" reset && goose -dir migrations postgres "$(DB_URL)" up
run:       ; DATABASE_URL="$(DB_URL)" go run ./cmd/minipay
test:      ; go test ./... -race -count=1