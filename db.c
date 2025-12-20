#include "ink.h"
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

const char db_file_name[] = "db.bin";

/*
 * Persistence behaviour:
 *  Record is durable on disk.
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
 * Reads a DiskRecord from filename at the given offset and copies
 * its value field into out_value.
 *
 * The offset must refer to the starting position of a valid DiskRecord
 * previously returned by db_insert.
 *
 * Persistence behaviour:
 *   out_value is updated with the value for the offset
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
  if (n != sizeof(DiskRecord) || dr.magic != RECORD_MAGIC) {
    close(fd);
    perror("corrup record");
    exit(1);
  }
  Record r = dr.record;
  strncpy(out_value, r.value, VALUE_SIZE);
  close(fd);
}

/*
 * Linear fallback lookup used on index miss.
 *
 * Persistence:
 *  Read-only; observes on-disk state.
 *
 * Failure behaviour:
 *  Returns -1 on I/O error.
 */
int db_get(const char *filename, const char *key, char *out_value) {
  int fd = open(filename, O_RDONLY);

  DiskRecord dr;
  int found = 0;

  while (1) {
    ssize_t n = read(fd, &dr, sizeof(DiskRecord));
    if (n == 0) break;

    if (n != sizeof(DiskRecord) || n < 0) {
      close(fd);
      return -1;
    }

    if (strncmp(dr.record.key, key, KEY_SIZE) == 0) {
      strncpy(out_value, dr.record.value, VALUE_SIZE);
      found = 1;
    }
  }
  close(fd);
  return found ? 0 : -1;
}

/*
 * Rewrites the database using the in-memory index as the source of truth.
 *
 * Persistence:
 *  Produces a new compacted database containing only the latest value
 *  per key, then atomically replaces the old file.
 *
 * Failure behaviour:
 *  On any error, exits without modifying the original database.
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
 * Writes a new entry to the database with DiskRecord.deleted = 1
 * Also removes the key from the in-memory index.
 *
 * Persistence behaviour:
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
 * Reconstructs the in-memory index by replaying the database log.
 * Latest record per key wins.
 *
 * Persistence:
 *  Read-only.
 *
 * Failure behaviour:
 *  Terminates the process on any I/O error.
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


