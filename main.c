#include "ink.h"
#include <stdio.h>


int main() {
  printf("Hi from Ink\n");
  build_index(db_file_name);
  compact();

  input_loop();
}
