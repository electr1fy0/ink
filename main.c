#include <errno.h>
#include <fcntl.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#define KEY_SIZE 32
#define VALUE_SIZE 256
#define MAX_ENTRIES 100

typedef struct {
  char key[KEY_SIZE];
  char value[VALUE_SIZE];
} Record;

typedef struct {
  char key[KEY_SIZE];
  off_t offset;
} IndexEntry;

// lseek(fd, offset, SEEK_SET)
// read(fd, &r, sizeof(Record));

IndexEntry db_insert(const char *filename, const char *key, const char *value) {
  int fd = open("db.bin", O_WRONLY | O_CREAT | O_APPEND, 0644);

  IndexEntry index_entry;
  Record r = {0};
  strncpy(r.key, key, KEY_SIZE - 1);
  strncpy(r.value, value, VALUE_SIZE - 1);

  ssize_t total = 0;
  while (total < sizeof(Record)) {
    ssize_t n = write(fd, ((char *)&r) + total, sizeof(Record) - total);
    if (n <= 0) exit(1);
    total += n;
  }
  close(fd);

  strncpy(index_entry.key, key, KEY_SIZE);

  return index_entry;
}

int db_get(const char *filename, const char *key, char *out_value) {
  int fd = open("db.bin", O_RDONLY);

  Record r;
  int found = 0;

  while (1) {
    ssize_t n = read(fd, &r, sizeof(Record));
    if (n == 0) break;

    if (n != sizeof(Record) || n < 0) {
      close(fd);
      return -1;
    }

    if (strncmp(r.key, key, KEY_SIZE) == 0) {
      strncpy(out_value, r.value, VALUE_SIZE);
      found = 1;
    }
  }
  close(fd);
  return found ? 0 : 1;
}

int return_index(const IndexEntry *indexes, char key[], int n) {
  for (int i = 0; i < n; ++i) {
    if (strncmp(indexes[i].key, key, KEY_SIZE) == 0) return indexes[i].offset;
  }
  return -1;
}

void input_loop() {
  char op[50];
  char key[KEY_SIZE];
  char value[VALUE_SIZE];
  do {
    printf("Enter operation: (insert, get)\n");
    scanf(" %s", op);
    printf("Operation entered: %s\n", op);
    printf("Enter key:\n");
    scanf("%s", key);

    if (strcmp(op, "get") == 0) {
      db_get("db.bin", key, value);
      printf("Key: %s, Value: %s\n", key, value);

    } else if (strcmp(op, "insert") == 0) {
      printf("Enter value for %s\n", key);
      scanf("%s", value);

      db_insert("db.bin", key, value);
      printf("Inserted\n");
    }

  } while (1);
}

int main() { input_loop(); }
