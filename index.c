#include "ink.h"
#include <stdlib.h>

IndexSlot *index_table;
int index_cap = 1024;
int index_count = 0;

static uint32_t hash_key(const char *key) {
  uint32_t h = 2166136261u; // FNV-1 32 bit hash offset
  for (int i = 0; i < KEY_SIZE && key[i]; ++i) {
    h ^= (unsigned char)key[i];
    h *= 16777619u; // a prime number used in 32-bit moduleo arithmetic
  }

  return h;
}

/*
 * Returns the on-disk offset for key.
 *  -1 if not found.
 */
off_t index_lookup(IndexSlot *idx_table, const char *key) {
  uint32_t h = hash_key(key);
  uint32_t idx = h % index_cap;

  for (int i = 0; i < index_cap; ++i) {
    IndexSlot *slot = &idx_table[idx];

    if (slot->used == 0)
      return -1;

    if (slot->used == 1 && strncmp(slot->key, key, KEY_SIZE) == 0)
      return slot->offset;

    idx = (idx + 1) % index_cap;
  }

  return -1;
}

/*
 * Removes key from the index by marking its slot as a tombstone.
 */
void index_delete(IndexSlot *idx_table, const char *key) {
  uint32_t h = hash_key(key);
  uint32_t idx = h % index_cap;

  for (int i = 0; i < index_cap; ++i) {
    IndexSlot *slot = &idx_table[idx];

    if (slot->used == 0)
      return;

    if (slot->used == 1 && strncmp(slot->key, key, KEY_SIZE) == 0) {
      slot->used = -1; // tombstone
      return;
    }

    idx = (idx + 1) % index_cap;
  }
}

void index_resize() {
  int new_cap = index_cap * 2;
  IndexSlot *new_index_table = calloc(new_cap, sizeof(IndexSlot));
  if (!new_index_table) {
    perror("index resize allocation failed");
    exit(1);
  }

  for (int i = 0; i < index_cap; ++i) {
    if (index_table[i].used == 1) {
      uint32_t h = hash_key(index_table[i].key);
      uint32_t idx = h % new_cap;
      
      while (new_index_table[idx].used != 0) {
        idx = (idx + 1) % new_cap;
      }
      
      new_index_table[idx] = index_table[i];
    }
  }

  IndexSlot *tmp = index_table;
  index_table = new_index_table;
  index_cap = new_cap;
  free(tmp);
}

/*
 * Inserts or updates a key
 * Reuses tombstones to preserve probe chains
 */
void index_update(IndexSlot *idx_table, const char *key, off_t offset) {
  if (idx_table == index_table && (float)index_count / index_cap > MAX_LOAD_FACTOR) {
    index_resize();
    idx_table = index_table; // Update local pointer after resize
  }

  uint32_t h = hash_key(key);
  uint32_t idx = h % index_cap;
  int first_tombstone = -1;

  for (int i = 0; i < index_cap; ++i) {
    IndexSlot *slot = idx_table + idx;

    if (slot->used == 1 && strncmp(slot->key, key, KEY_SIZE) == 0) {
      slot->offset = offset;
      return;
    }

    if (slot->used == -1 && first_tombstone == -1) {
      first_tombstone = idx;
    }

    if (slot->used == 0) {
      if (first_tombstone != -1) {
        slot = &idx_table[first_tombstone];
      } else {
        index_count++;
      }

      strncpy(slot->key, key, KEY_SIZE - 1);
      slot->offset = offset;
      slot->used = 1;
      return;
    }
    idx = (idx + 1) % index_cap;
  }
}