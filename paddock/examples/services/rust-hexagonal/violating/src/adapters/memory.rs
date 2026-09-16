use crate::domain::order::Order;
use crate::ports::repository::Repository;

pub struct MemoryRepository;

impl Repository for MemoryRepository {
    fn find(&self, _identifier: &str) -> Option<&str> {
        None
    }
}

fn _example(_order: Order) {}
