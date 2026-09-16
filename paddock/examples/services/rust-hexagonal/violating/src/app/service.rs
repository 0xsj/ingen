use crate::domain::order::Order;
use crate::ports::repository::Repository;

pub struct Service<R: Repository> {
    repository: R,
    _order: Option<Order>,
}
