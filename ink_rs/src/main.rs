use std::collections::HashMap;
use std::fs::{File, OpenOptions};
use std::io::{self, BufRead, BufReader, Seek, SeekFrom, Write};
use std::path::Path;

struct KVStore {
    index: HashMap<String, u64>,
    file: File,
    dead_bytes: usize,
    compaction_threshold: usize,
}

impl KVStore {
    fn open(path: impl AsRef<Path>) -> io::Result<Self> {
        let mut file = OpenOptions::new()
            .create(true)
            .read(true)
            .append(true)
            .open(path)?;
        let index = Self::build_index(&mut file)?;

        Ok(KVStore {
            index,
            file,
            dead_bytes: 0,
            compaction_threshold: 1024,
        })
    }

    fn build_index(file: &mut File) -> io::Result<HashMap<String, u64>> {
        let mut index = HashMap::new();

        file.seek(SeekFrom::Start(0))?;
        let reader = BufReader::new(file.try_clone()?);

        let mut offset = 0u64;
        for line in reader.lines() {
            let line = line?;
            let line_bytes = line.len() as u64 + 1;
            if let Some((key, _)) = line.split_once('\t') {
                index.insert(key.to_string(), offset);
            }
            offset += line_bytes;
        }
        Ok(index)
    }

    fn set(&mut self, key: String, value: String) -> io::Result<()> {
        let offset = self.file.seek(SeekFrom::End(0))?;
        let entry = format!("{}\t{}\n", key, value);
        self.file.write_all(entry.as_bytes())?;

        if let Some(&old_offset) = self.index.get(&key) {
            self.file.seek(SeekFrom::Start(old_offset))?;
            let mut reader = BufReader::new(&self.file);
            let mut old_line = String::new();
            reader.read_line(&mut old_line)?;
            self.dead_bytes += old_line.len();
        }

        self.file.seek(SeekFrom::End(0))?;
        self.file.write_all(entry.as_bytes())?;
        self.index.insert(key, offset);

        if self.dead_bytes > self.compaction_threshold {
            self.compact()?;
        }

        Ok(())
    }

    fn get(&mut self, key: &str) -> io::Result<Option<String>> {
        let offset = match self.index.get(key) {
            Some(&off) => off,
            None => return Ok(None),
        };

        self.file.seek(SeekFrom::Start((offset)))?;
        let mut reader = BufReader::new(&self.file);
        let mut line = String::new();
        reader.read_line(&mut line)?;

        Ok(line.split_once('\t').map(|(_, v)| v.trim().to_string()))
    }

    fn compact(&mut self) -> io::Result<()> {
        let temp_path = "temp.db";
        let mut temp_file = File::create(temp_path)?;
        let mut new_index = HashMap::new();

        let keys: Vec<_> = self.index.keys().cloned().collect();
        for key in keys {
            if let Some(value) = self.get(&key)? {
                let offset = temp_file.stream_position()?;
                writeln!(temp_file, "{}\t{}", key, value)?;
                new_index.insert(key.clone(), offset);
            }
        }

        std::fs::rename(temp_path, "store.db")?;

        self.file = OpenOptions::new()
            .read(true)
            .append(true)
            .open("store.db")?;
        self.index = new_index;
        self.dead_bytes = 0;

        Ok(())
    }
}

fn main() -> io::Result<()> {
    let mut store = KVStore::open("store.db")?;

    store.set("name".to_string(), "meow".to_string())?;
    store.set("age".to_string(), "420".to_string())?;
    store.set("age".to_string(), "0x45".to_string())?;

    println!("{:?}", store.get("name"));
    println!("{:?}", store.get("age"));

    Ok(())
}
