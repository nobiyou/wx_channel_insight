package normalize

func cloneSnapshot(source Snapshot) Snapshot {
	cloned := source
	cloned.ContentSnapshots = append([]ContentSnapshot(nil), source.ContentSnapshots...)
	cloned.CommentSnapshots = append([]CommentSnapshot(nil), source.CommentSnapshots...)
	cloned.TopicObservations = append([]TopicObservation(nil), source.TopicObservations...)
	cloned.AccountObservations = append([]AccountObservation(nil), source.AccountObservations...)

	for i := range cloned.ContentSnapshots {
		cloned.ContentSnapshots[i].Tags = append([]string(nil), cloned.ContentSnapshots[i].Tags...)
		cloned.ContentSnapshots[i].Keywords = append([]string(nil), cloned.ContentSnapshots[i].Keywords...)
	}
	for i := range cloned.CommentSnapshots {
		cloned.CommentSnapshots[i].Keywords = append([]string(nil), cloned.CommentSnapshots[i].Keywords...)
	}
	for i := range cloned.TopicObservations {
		cloned.TopicObservations[i].Keywords = append([]string(nil), cloned.TopicObservations[i].Keywords...)
	}
	for i := range cloned.AccountObservations {
		cloned.AccountObservations[i].Tags = append([]string(nil), cloned.AccountObservations[i].Tags...)
	}

	return cloned
}
