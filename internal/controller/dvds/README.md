# Distributed Volatile Data Structure Controller

The "distributed volatile data structure" or `dvds` controller is an `endocrine`
signal transduction mechanism for `volatile` data types.

It is responsible for collaborating with other nodes in order to maintain
a set of consistent data structures using a Write-Ahead Log.

There are 2 types of log entries:

- Standard log entries.
- Snapshot entries.

When the WAL becomes large, nodes can decide on a snapshot entry. The snapshot
entry contains the whole datastructure in binary format. Its content is not
persisted in the WAL but decoded and becomes the new data structure.

`Endocrine` pathways are inherently slower and have a strong emphasis on
consistency. Thus, when watching a `dvds` the observer will obtain the whole new
state of the data structure at once. This allow a stateless usage, or in other
words the observer won't have to maintain a duplicate state of the data structure.

The downsides are increased allocations and O(1) data duplication. To address
these inconveniences the user can choose to shard their data structures if they
become too large.

Question:

- How do we keep a clean state of the snapshot and provide up-to-date data
  structures to the user?
  - Should we copy the snapshot and replay the whole WAL everytime a new entry
    is logged?
  - Or should we create CoW views of the snapshot that can easily be duplicated and
    shared with the observer(s)?

Question:

- If we're watching the consensus/WAL, how do we know we have the correct sequence
  of the WAL and that we are not missing any entry?
- If we are missing entries, how do we request the one we missed.

- Can we compute hash of each entries based on the value of the hash of the previous
  entry?
