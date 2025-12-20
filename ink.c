#include "ink.h"
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

const char db_file_name[] = "db.bin";
IndexSlot index_table[INDEX_CAPACITY];

void index_update(const char *key, off_t offset);
void index_delete(const char *key);
off_t index_lookup(const char *key);

/*
 * db_insert
 *
 * Appends to DiskRecord to the end of filename and fsyncs it.
 * Returns an IndexEntry pointing to the on-disk offset.
 *
 * Persistence:
 *  On successful return, the record is durable on disk.
 *
 * Failure behaviour:
 *  Terminates the process on any I/O error.
 */
void db_insert(const char *filename, const char *key, const char *value) {
  int fd = open(filename, O_WRONLY | O_CREAT | O_APPEND, 0644);
  if (fd < 0) {
    perror("open");
    exit(1);
  }

  Record r = {0};
  strncpy(r.key, key, KEY_SIZE - 1);
  strncpy(r.value, value, VALUE_SIZE - 1);

  DiskRecord dr = {.magic = RECORD_MAGIC, .record = r};

  off_t offset = lseek(fd, 0, SEEK_END);
  if (offset == (off_t)-1) {
    perror("lseek");
    close(fd);
    exit(1);
  }

  ssize_t total = 0;
  while (total < (ssize_t)sizeof(DiskRecord)) {
    ssize_t n = write(fd, ((char *)&dr) + total, sizeof(DiskRecord) - total);
    if (n <= 0) {
      perror("write");
      close(fd);
      exit(1);
    }

    total += n;
  }
  if (fsync(fd) < 0) {
    perror("fsync");
    close(fd);
    exit(1);
  }

  close(fd);

  index_update(key, offset);
}
/*
 * db_get_at
 *
 * Reads a DiskRecord from filename at the given offset and copies
 * its value field into out_value.
 *
 * The offset must refer to the starting position of a valid DiskRecord
 * previously returned by db_insert. No validation is performed beyond
 * the read itself.
 *
 * Persistence:
 *  The read observes the on-disk state at the time of the call.
 *
 * Failure behaviour:
 *  Terminates the process on any I/O error.
 *  Undefined if offset does not point to a valid DiskRecord.
 *
 * Ownership:
 *  out_value must point to a writable buffer of at least VALUE_SIZE bytes.
 */
void db_get_at(const char *filename, off_t offset, char *out_value) {
  int fd = open(filename, O_RDONLY);
  DiskRecord dr;

  lseek(fd, offset, SEEK_SET);
  ssize_t n = read(fd, &dr, sizeof(DiskRecord));
  if (n < 0) {
    close(fd);
    perror("read");
    exit(1);
  }
  Record r = dr.record;
  strncpy(out_value, r.value, VALUE_SIZE);
  close(fd);
}

/*
 * db_get
 *
 * Scans the database file and matches the keys of the records with the
 * param key and then stores the value in out_value
 *
 * Persistence:
 *  Performs read only I/O and does not modify on-disk state.
 *
 * Failure behaviour:
 *  Closes fd and terminates the program on any I/O error.
 *
 * Preconditions:
 *  Out value must be a write only buffer with sufficient space
 */
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
  return found ? 0 : -1;
}


/*
 * compact
 *
 * Performs disk compaction on the database by rewriting all records to a
 * new compact file from the in-memory index and then
 * replaces the old database file
 *
 * Persistence:
 * A new database file is created with the same name with
 * redundant unused value entries removed and only latest values retained
 * per key entry
 *
 * Failure behaviour:
 * Exits the program on any I/O error. Original database is left intact
 */
void compact() {
  int in_fd = open(db_file_name, O_RDONLY);
  if (in_fd < 0) {
    perror("open db");
    exit(1);
  }

  int out_fd = open("db.bin.compact", O_WRONLY | O_TRUNC | O_CREAT, 0644);
  if (out_fd < 0) {
    perror("open compact");
    close(in_fd);
    exit(1);
  }

  DiskRecord dr;
  off_t new_offset = 0;

  for (int i = 0; i < INDEX_CAPACITY; ++i) {
    if (index_table[i].used != 1) continue;
    if (lseek(in_fd, index_table[i].offset, SEEK_SET) == (off_t)-1) {
      perror("lseek");
      close(in_fd);
      close(out_fd);
      exit(1);
    }

    ssize_t n = read(in_fd, &dr, sizeof(DiskRecord));
    if (n != sizeof(DiskRecord) || dr.magic != RECORD_MAGIC) {
      fprintf(stderr, "corrup record during compaction\n");
      close(in_fd);
      close(out_fd);
      exit(1);
    }

    ssize_t written = write(out_fd, &dr, sizeof(DiskRecord));
    if (written != sizeof(DiskRecord)) {
      perror("write compact");
      close(in_fd);
      close(out_fd);
      exit(1);
    }

    // update index to new physical layout
    index_table[i].offset = new_offset;
    new_offset += sizeof(DiskRecord);
  }

  if (fsync(out_fd) < 0) {
    perror("fsync");
    exit(1);
  }

  close(out_fd);
  close(in_fd);

  if (rename("db.bin.compact", "db.bin") < 0) {
    perror("rename");
    exit(1);
  }
}

/*
 * hash_key
 *
 * generates the hash-key using the FNV hashing algo
 */
static uint32_t hash_key(const char *key) {
  uint32_t h = 2166136261u; // FNV-1 32 bit hash offset
  for (int i = 0; i < KEY_SIZE && key[i]; ++i) {
    h ^= (unsigned char)key[i];
    h *= 16777619u; // a prime number used in 32-bit moduleo arithmetic
  }

  return h;
}

/*
 * index_lookup
 *
 * returns the offset of the key from the in-memory index
 *  -1 =  if not found
 *  >=0 = offset value
 */
off_t index_lookup(const char *key) {
  uint32_t h = hash_key(key);
  uint32_t idx = h % INDEX_CAPACITY;

  for (int i = 0; i < INDEX_CAPACITY; ++i) {
    IndexSlot *slot = &index_table[idx];

    // unused slot (offset not found)
    if (slot->used == 0) return -1;

    // used slot and not tombstone
    if (slot->used == 1 && strncmp(slot->key, key, KEY_SIZE) == 0) return slot->offset;

    idx = (idx + 1) % INDEX_CAPACITY;
  }

  return -1;
}



/*
 * index_delete
 *
 * Marks used slot as a tombstone in the index table
 */
void index_delete(const char *key) {
  uint32_t h = hash_key(key);
  uint32_t idx = h % INDEX_CAPACITY;

  for (int i = 0; i < INDEX_CAPACITY; ++i) {
    IndexSlot *slot = &index_table[idx];

    if (slot->used == 0) return;

    if (slot->used == 1 && strncmp(slot->key, key, KEY_SIZE) == 0) {
      slot->used = -1; // tombstone
      return;
    }

    idx = (idx + 1) % INDEX_CAPACITY;
  }
}

/*
 * index_update
 *
 * Updates offset for a key
 */
void index_update(const char *key, off_t offset) {
  uint32_t h = hash_key(key);
  uint32_t idx = h % INDEX_CAPACITY;
  int first_tombstone = -1;

  for (int i = 0; i < INDEX_CAPACITY; ++i) {
    IndexSlot *slot = index_table + idx;

    if (slot->used == 1 && strncmp(slot->key, key, KEY_SIZE) == 0) {
      slot->offset = offset;
      return;
    }

    if (slot->used == -1 && first_tombstone == -1) { first_tombstone = idx; }

    if (slot->used == 0) {
      if (first_tombstone != -1) slot = &index_table[first_tombstone];

      strncpy(slot->key, key, KEY_SIZE - 1);
      slot->offset = offset;
      slot->used = 1;
      return;
    }
    idx = (idx + 1) % INDEX_CAPACITY;
  }
}

/*
 * db_delete
 *
 * Writes a new entry to the database with DiskRecord.deleted = 1
 * Also updates the index entry offset to -1
 *
 * Persistence:
 *  On succesful return, the tombstone record is durable on disk.
 *
 * Failure behaviour:
 *  Terminates the process on any I/O error.
 */
void db_delete(const char *filename, const char *key) {
  int fd = open(filename, O_WRONLY | O_CREAT | O_APPEND, 0644);
  if (fd < 0) {
    perror("open");
    exit(1);
  }

  off_t offset = lseek(fd, 0, SEEK_END);
  if (offset == (off_t)-1) {
    perror("lseek");
    exit(1);
  }

  DiskRecord dr = {0};
  dr.magic = RECORD_MAGIC;
  dr.deleted = 1;
  strncpy(dr.record.key, key, KEY_SIZE - 1);

  ssize_t written = write(fd, &dr, sizeof(DiskRecord));
  if (written != sizeof(DiskRecord)) {
    perror("write");
    exit(1);
  }

  if (fsync(fd) < 0) {
    perror("fsync");
    close(fd);
    exit(1);
  }
  close(fd);

  index_delete(key);
}

/*
 * build_index
 *
 * Reads the database file and builds the in-memory global index of all records
 * Takes the latest value for each key
 *
 * Persistence:
 *  Performs read-only I/O and does not modify on-disk state.
 *
 * Failure behaviour:
 *  Terminates the process on any I/O error.
 *
 *
 */
void build_index(const char *filename) {
  int fd = open(filename, O_RDONLY | O_CREAT, 0644);
  if (fd < 0) {
    perror("open");
    exit(1);
  }

  memset(index_table, 0, sizeof(index_table));

  DiskRecord dr;
  off_t offset = 0;

  while ((read(fd, &dr, sizeof(DiskRecord))) == sizeof(DiskRecord)) {
    if (dr.magic != RECORD_MAGIC) break;

    if (dr.deleted) {
      index_delete(dr.record.key);
    } else {
      index_update(dr.record.key, offset);
    }

    offset += sizeof(DiskRecord);
  }

  close(fd);
}

/*
 * input_loop
 *
 * Continous user facing input loop for testing the program
 * Breaks on Ctrl-C
 */

void input_loop() {
  char op[50];
  char key[KEY_SIZE];
  char in_value[VALUE_SIZE];
  char out_value[VALUE_SIZE];

  while (1) {
    printf("Enter operation: (insert, get, delete)\n");
    if (scanf(" %s", op) != 1) break;
    printf("Enter key:\n");
    if (scanf("%s", key) != 1) break;

    int found = 0;
    if (strcmp(op, "get") == 0) {
      off_t offset = index_lookup(key);

      if (offset < 0) {
        printf("index miss\n");
        if (db_get(db_file_name, key, out_value) >= 0) found = 1;
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
