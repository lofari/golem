(ns golem.dsl.engine.output
  "Parse session output: structured EDN + filesystem diff."
  (:require [clojure.java.io :as io]
            [clojure.edn :as edn]
            [clojure.set]))

(defn read-session-output
  "Read session-output.edn from working directory. Returns {} if missing."
  [working-dir]
  (let [f (io/file working-dir "session-output.edn")]
    (if (.exists f)
      (edn/read-string (slurp f))
      {})))

(defn scan-files
  "Return set of relative file paths in directory (non-recursive, top-level)."
  [dir]
  (->> (io/file dir)
       (.listFiles)
       (filter #(.isFile %))
       (map #(.getName %))
       set))

(defn file-diff
  "Return files present in after but not in before."
  [before-files after-files]
  (vec (clojure.set/difference after-files before-files)))

(defn collect-output
  "Collect session output: merge structured EDN with file diff metadata."
  [working-dir before-files contract]
  (let [structured (read-session-output working-dir)
        after-files (scan-files working-dir)
        new-files (file-diff before-files after-files)]
    (cond-> structured
      (and (seq new-files) (contains? (set (:writes contract)) :code))
      (assoc-in [:code :files] new-files))))
