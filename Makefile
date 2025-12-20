ink: main.c db.c index.c
	clang -Wall -Wextra -std=c11 main.c db.c index.c -o ink

run: ink
	./ink

clean:
	rm -f ink