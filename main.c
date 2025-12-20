#include "ink.h"

int main() {
  build_index(db_file_name);
  compact();
  printf("Hi from Ink\n");
  input_loop();
}
