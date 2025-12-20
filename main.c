#include "ink.h"

void input_loop() {
  char op[50];
  char key[KEY_SIZE];
  char in_value[VALUE_SIZE];
  char out_value[VALUE_SIZE];

  index_table = malloc(sizeof(IndexSlot) * index_cap);

  while (1) {
    printf("Enter operation: (insert, get, delete)\n");
    if (scanf(" %s", op) != 1)
      break;
    printf("Enter key:\n");
    if (scanf("%s", key) != 1)
      break;

    int found = 0;
    if (strcmp(op, "get") == 0) {
      off_t offset = index_lookup(index_table,  key);

      if (offset < 0) {
        printf("index miss\n");
        if (db_get(db_file_name, key, out_value) >= 0)
          found = 1;
      } else {
        printf("index hit\n");
        db_get_at(db_file_name, offset, out_value);
        found = 1; // since it's an index hit
      }
      if (found)
        printf("Key: %s, Value: %s\n", key, out_value);
      else
        printf("Value does not exit for key: %s\n", key);
    } else if (strcmp(op, "insert") == 0) {
      printf("Enter value for %s\n", key);
      scanf("%s", in_value);

      db_insert(db_file_name, key, in_value);
      printf("Inserted %s\n", key);
    } else if (strcmp(op, "delete") == 0) {
      db_delete(db_file_name, key);
      printf("Key %s deleted\n", key);
    }
  }
}

int main() {
  printf("Hi from Ink\n");
  build_index(db_file_name);
  compact();

  input_loop();
}
