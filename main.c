#include <stdio.h>
#include <string.h>

#define KEY_SIZE 32
#define VALUE_SIZE 256

typedef struct {
  char key[KEY_SIZE];
  char value[VALUE_SIZE];
} Record;

int db_insert(const char *filename, const char *key, const char *value) {
  FILE *fp = fopen(filename, "abc");
  if (!fp) return -1;

  Record r = {0};
  strncpy(r.key, key, KEY_SIZE - 1);
  strncpy(r.value, value, VALUE_SIZE - 1);

  fwrite(&r, sizeof(Record), 1, fp);
  fclose(fp);

  return 0;
}

int db_get(const char *filename, const char *key, char *out_value) {
  FILE *fp = fopen(filename, "rb");
  if (!fp) return -1;

  Record r;

  while (fread(&r, sizeof(Record), 1, fp)) {
    if (strncmp(r.key, key, KEY_SIZE) == 0) {
      strncpy(out_value, r.value, VALUE_SIZE);
      fclose(fp);
      return 0;
    }
  }
  fclose(fp);
  return -1;
}
#include <stdio.h>

int main() {
    db_insert("db.bin", "name", "ayush");
    db_insert("db.bin", "lang", "c");

    char value[VALUE_SIZE];
    if (db_get("db.bin", "lang", value) == 0) {
        printf("lang = %s\n", value);
    }
}
