#include "ink.h"


IndexEntry db_insert(const char *filename, const char *key, const char *value) {
  int fd = open("db.bin", O_WRONLY | O_CREAT | O_APPEND, 0644);

  IndexEntry index_entry = {0};
  Record r = {0};
  strncpy(r.key, key, KEY_SIZE - 1);
  strncpy(r.value, value, VALUE_SIZE - 1);

  off_t offset = lseek(fd, 0, SEEK_END);
  if (offset == (off_t)-1) {
    perror("lseek");
    close(fd);
    exit(1);
  }

  ssize_t total = 0;
  while (total < sizeof(Record)) {
    ssize_t n = write(fd, ((char *)&r) + total, sizeof(Record) - total);
    if (n <= 0) {
      perror("write");
      close(fd);
      exit(1);
    }

    total += n;
  }

  close(fd);

  index_entry.offset = offset;
  strncpy(index_entry.key, key, KEY_SIZE);
  return index_entry;
}

void db_get_at(const char *filename, off_t offset, char *out_value) {
  int fd = open(filename, O_RDONLY);
  Record r;

  lseek(fd, offset, SEEK_SET);

  ssize_t n = read(fd, &r, sizeof(Record));
  if (n < 0) {
    close(fd);
    perror("read");
    exit(1);
  }
  strncpy(out_value, r.value, VALUE_SIZE);
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
  IndexEntry index_entry;
  do {
    printf("Enter operation: (insert, get)\n");
    scanf(" %s", op);
    printf("Operation entered: %s\n", op);

    if (strcmp(op, "get") == 0) {
      char method[50];
      printf("Enter method for get: (index / key)%s\n", key);
      scanf("%s", method);

      if (strcmp(method, "index") == 0) {
        char out_value[VALUE_SIZE];
        db_get_at("db.bin", index_entry.offset, out_value);

        printf("Value at %lld offset with key = %s is %s\n", index_entry.offset, index_entry.key, out_value);
      } else if (strcmp(method, "key") == 0) {
        printf("Enter key:\n");
        scanf("%s", key);
        db_get("db.bin", key, value);
        printf("Key: %s, Value: %s\n", key, value);
      }

    } else if (strcmp(op, "insert") == 0) {
      printf("Enter key:\n");
      scanf("%s", key);
      printf("Enter value for %s\n", key);
      scanf("%s", value);

      index_entry = db_insert("db.bin", key, value);
      printf("Inserted\n");
    }

  } while (1);
}
