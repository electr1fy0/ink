#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>

#define KEY_SIZE 32
#define VALUE_SIZE 256

typedef struct {
  char key[KEY_SIZE];
  char value[VALUE_SIZE];
} Record;

int db_insert(const char *filename, const char *key, const char *value) {
  int fd = open("db.bin", O_WRONLY | O_CREAT | O_APPEND, 0644);

  Record r = {0};
  strncpy(r.key, key, KEY_SIZE - 1);
  strncpy(r.value, value, VALUE_SIZE - 1);

  ssize_t total = 0;
  while (total < sizeof(Record)) {
    ssize_t n = write(fd, ((char *)&r) + total, sizeof(Record) - total);
    if (n <= 0) return -1;
    total += n;
  }
  close(fd);
  return 0;
}

int db_get(const char *filename, const char *key, char *out_value) {

  int fd = open("db.bin", O_RDONLY);

  Record r;

  while (1) {
    ssize_t n = read(fd, &r, sizeof(Record));
    if (n == 0) break;
    if (n < 0) return -1;

    if (n != sizeof(Record) || n < 0) {
      close(fd);
      return -1;
    }

    if (strncmp(r.key, key, KEY_SIZE) == 0) {
      strncpy(out_value, r.value, VALUE_SIZE);
      close(fd);
      return 0;
    }
  }
  close(fd);
  return -1;
}

int main() {
  db_insert("db.bin", "name", "ayush");
  db_insert("db.bin", "lang", "c");

  char value[VALUE_SIZE];
  if (db_get("db.bin", "lang", value) == 0) { printf("lang = %s\n", value); }
}
