use crate::adapters::memory::MemoryRepository;
use std::fmt;

pub struct Order {
    pub identifier: String,
    _display: Option<fmt::Error>,
    _repository: Option<MemoryRepository>,
}
