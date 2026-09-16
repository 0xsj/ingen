pub trait Repository {
    fn find(&self, identifier: &str) -> Option<&str>;
}
