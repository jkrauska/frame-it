BINARY := frame-it
CMD    := ./cmd/frame-it

.PHONY: all clean run

all:
	go build -o $(BINARY) $(CMD)

clean:
	rm -f $(BINARY)

run: all
	./$(BINARY) $(ARGS)
