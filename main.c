#include "ink.h"

int main() {
  build_index(db_file_name);
  for (int i = 0; i < index_entry_len; ++i) { printf("%s %lld\n", index_entries[i].key, index_entries[i].offset); }
  compact();
  printf("Hi from Ink\n");
  input_loop();
}
