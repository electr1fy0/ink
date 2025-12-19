ink: main.c ink.c
	clang -Wall -Wextra -std=c11 main.c ink.c -o ink

run: ink
	./ink

clean:
	rm -f ink
