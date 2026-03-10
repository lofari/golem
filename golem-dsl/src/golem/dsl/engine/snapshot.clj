(ns golem.dsl.engine.snapshot
  "Versioned state persistence as EDN files."
  (:require [clojure.java.io :as io]
            [clojure.edn :as edn]))

(defn run-dir [base-dir run-id]
  (io/file base-dir "runs" (name run-id)))

(defn save-state!
  "Write state to runs/<run-id>/state-v<version>.edn"
  [base-dir run-id version state]
  (let [dir (run-dir base-dir run-id)
        file (io/file dir (str "state-v" version ".edn"))]
    (.mkdirs dir)
    (spit file (pr-str state))))

(defn load-state
  "Read state from runs/<run-id>/state-v<version>.edn"
  [base-dir run-id version]
  (let [file (io/file (run-dir base-dir run-id)
                      (str "state-v" version ".edn"))]
    (when (.exists file)
      (edn/read-string (slurp file)))))

(defn save-log!
  "Append a log entry to runs/<run-id>/log.edn"
  [base-dir run-id entry]
  (let [dir (run-dir base-dir run-id)
        file (io/file dir "log.edn")
        existing (if (.exists file)
                   (edn/read-string (slurp file))
                   [])]
    (.mkdirs dir)
    (spit file (pr-str (conj existing entry)))))

(defn save-graph!
  "Write the agent graph to runs/<run-id>/graph.edn"
  [base-dir run-id graph]
  (let [dir (run-dir base-dir run-id)
        file (io/file dir "graph.edn")]
    (.mkdirs dir)
    (spit file (pr-str graph))))

(defn next-run-id
  "Generate the next run ID by scanning existing runs."
  [base-dir]
  (let [runs-dir (io/file base-dir "runs")]
    (if (.exists runs-dir)
      (let [existing (->> (.listFiles runs-dir)
                          (filter #(.isDirectory %))
                          (map #(.getName %))
                          (filter #(re-matches #"run-\d+" %))
                          (map #(Integer/parseInt (subs % 4)))
                          sort)]
        (keyword (str "run-" (format "%03d" (inc (or (last existing) 0))))))
      :run-001)))
