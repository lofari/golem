(ns golem.dsl.errors.types)

(def defaults
  {:transient          {:action :retry :max 3}
   :malformed-output   {:action :re-run :max 2}
   :contract-violation {:action :snapshot-and-halt}
   :unrecoverable      {:action :snapshot-and-halt}})

(defn classify
  "Classify an error condition into an error type."
  [{:keys [exit-code output-missing schema-mismatch spawn-failed timeout]}]
  (cond
    spawn-failed    :unrecoverable
    timeout         :transient
    (and exit-code (not= 0 exit-code)) :transient
    output-missing  :malformed-output
    schema-mismatch :malformed-output
    :else           :unrecoverable))
