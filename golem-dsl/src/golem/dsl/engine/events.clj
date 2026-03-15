(ns golem.dsl.engine.events
  "NDJSON event emission for Go binary consumption."
  (:require [clojure.data.json :as json]))

(defn emit! [event-type data]
  (println (json/write-str (assoc data :type (name event-type)))))
