#include "ink.h"

const char db_file_name[] = "db.bin";

IndexEntry index_entries[MAX_ENTRIES];
int index_entry_len = 0;

IndexEntry db_insert(const char *filename, const char *key, const char *value) {
  int fd = open(filename, O_WRONLY | O_CREAT | O_APPEND, 0644);

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
  while (total < (ssize_t)sizeof(Record)) {
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

void build_index(const char *filename) {
  int fd = open(filename, O_RDONLY);
  Record r;
  ssize_t n;
  off_t offset = lseek(fd, 0, SEEK_SET);
  do {
    n = read(fd, &r, sizeof(Record));
    if (n < 0) {
      perror("read");
      close(fd);
      exit(1);
    }

    if (n == sizeof(r)) {
      strcpy(index_entries[index_entry_len].key, r.key);
      index_entries[index_entry_len].offset = offset;
    }
  } while (n > 0);
}

int db_get(const char *filename, const char *key, char *out_value) {
  int fd = open(filename, O_RDONLY);

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

int get_offset(char *key) {
  int found_offset = -1;
  for (int i = 0; i < index_entry_len; ++i) {
    if (strncmp(index_entries[i].key, key, KEY_SIZE) == 0) found_offset = index_entries[i].offset;
  }
  return found_offset;
}

void input_loop() {
  char op[50];
  char key[KEY_SIZE];
  char in_value[VALUE_SIZE];
  char out_value[VALUE_SIZE];

  while (1) {
    printf("Enter operation: (insert, get)\n");
    scanf(" %s", op);
    printf("Enter key:\n");
    scanf("%s", key);

    if (strcmp(op, "get") == 0) {
      off_t offset = get_offset(key);
      if (offset < 0) {
        db_get(db_file_name, key, out_value);
      } else {
        db_get_at(db_file_name, offset, out_value);
      }
      printf("Key: %s, Value: %s\n", key, out_value);

    } else if (strcmp(op, "insert") == 0) {
      printf("Enter value for %s\n", key);
      scanf("%s", in_value);

      index_entries[index_entry_len++] = db_insert(db_file_name, key, in_value);
      printf("Inserted %s\n", key);
    }
  }
}
