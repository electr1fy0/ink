ink: main.c ink.c
	clang main.c ink.c -o ink

run: ink
	./ink

clean:
	rm -f ink
